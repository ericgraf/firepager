#!/usr/bin/env python3
"""Publish a compact rotation-state snapshot to RabbitMQ's receiver_status queue."""
import json
import os
import subprocess
import sys
from datetime import datetime, timezone

import pika


def broker_url():
    value = os.environ.get("RABBITMQ_URL")
    if value:
        return value
    # Reuse the existing client connection configuration without writing its
    # credential-bearing URL to disk or command output.
    for container in ("firepager-host-1", "firepager-client-host"):
        result = subprocess.run(
            ["docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", container],
            text=True, capture_output=True, check=False, timeout=5,
        )
        if result.returncode == 0:
            for line in result.stdout.splitlines():
                if line.startswith("RABBITMQ_URL="):
                    return line.split("=", 1)[1]
    raise RuntimeError("RABBITMQ_URL is unavailable (no configured client container)")


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: publish_receiver_status.py STATE_JSON INVENTORY_JSON")
    with open(sys.argv[1], encoding="utf-8") as stream:
        state = json.load(stream)
    with open(sys.argv[2], encoding="utf-8") as stream:
        inventory = json.load(stream)
    active = state.get("active_detector")
    observed = datetime.now(timezone.utc).isoformat()
    params = pika.URLParameters(broker_url())
    params.socket_timeout = 5
    connection = pika.BlockingConnection(params)
    try:
        channel = connection.channel()
        channel.queue_declare(queue="receiver_status", durable=True)
        channel.confirm_delivery()
        for receiver_id, config in inventory.get("receivers", {}).items():
            receiver = state.get("receivers", {}).get(receiver_id, {})
            is_active = receiver_id == active
            status = "error" if receiver.get("status") in {"error", "unresolved"} else "idle"
            snapshot = {
                "receiver_id": receiver_id,
                "host": config.get("host", ""),
                "status": "error" if status == "error" else "detector" if is_active else status,
                "role": "detector" if is_active else "legacy",
                "frequency_hz": receiver.get("current_frequency_hz", config.get("tuner_frequency_hz")),
                "gain_db": receiver.get("current_gain_db", config.get("gain_db")),
                "ppm": receiver.get("current_ppm", config.get("ppm")),
                "active_detector": is_active,
                "decoded_messages_count": int(state.get("decoded_messages_count", 0)),
                "calibrations_accepted_count": int(state.get("calibrations_accepted_count", 0)),
                "last_calibration_utc": receiver.get("last_calibration_utc"),
                "observed_at": observed,
            }
            if not channel.basic_publish(
                exchange="", routing_key="receiver_status",
                body=json.dumps(snapshot, separators=(",", ":")),
                properties=pika.BasicProperties(delivery_mode=2, content_type="application/json"),
                mandatory=True,
            ):
                raise RuntimeError("broker did not confirm receiver snapshot")
    finally:
        connection.close()


if __name__ == "__main__":
    main()

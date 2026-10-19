import argparse
import base64
import json
import os
import random
import re
import shutil
import subprocess
import time
import urllib.request
from datetime import datetime, timezone

import pika
from pika.exceptions import AMQPError, ConnectionWrongStateError, StreamLostError


RAW_CALL_EXAMPLES = [
    "POCSAG512: Address:  930032  Function: 2  Alpha:   ID:gdsJ3D<LF>SMOKE VISIBLE INSIDE-FIRE <LF>15:55:48<LF>6525 BANK ST  Dv: 9  Sec: 91D <LF>Gd: 91D<LF>CENOTE RD & STONE SCHOOL RD<LF>PT91 PT93 P92 LA91 TA92 TA94<LF>TA83 SC90 #36719<LF>Sent by: FDP2<EOT><NUL>",
    "POCSAG512: Address:  910000  Function: 3  Alpha:   ID:IMzuFU<LF>2ND PAGE STRUCTURE FIRE AT 6525<LF>BANK ST<LF>Sent by: FDP1<EOT><NUL>",
    "POCSAG512: Address:    1296  Function: 2  Alpha:   ID:0VFZuQ<LF>MVC-ROLLOVER  16:35:53<LF>3168 DALMAC RD  Dv: 9  Sec: 92B<LF> Gd: 92B<LF>FOREST RD & SPRINGHILL RD<LF>P92 RE93 SO23 TA92 SC90 #37196<LF>Sent by: FDP1<EOT>",
]

OLLAMA_HOST = os.environ.get("OLLAMA_HOST", "http://127.0.0.1:11434")
OLLAMA_CONTAINER_NAME = os.environ.get("OLLAMA_CONTAINER_NAME", "firepager-ollama")
OLLAMA_DOCKER_IMAGE = os.environ.get("OLLAMA_DOCKER_IMAGE", "ollama/ollama")
RABBITMQ_SECRET_NAME = os.environ.get("RABBITMQ_SECRET_NAME", "firepager-rmq-default-user")
RABBITMQ_SECRET_NAMESPACES = [
    namespace.strip()
    for namespace in os.environ.get("RABBITMQ_SECRET_NAMESPACES", "default,flux-system").split(",")
    if namespace.strip()
]


def log_status(message):
    print(f"[simulator] {message}", flush=True)


def dedupe(values):
    seen = set()
    output = []
    for value in values:
        if value in seen:
            continue
        seen.add(value)
        output.append(value)
    return output


def get_local_rabbitmq_url():
    return os.environ.get("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")


def get_rabbitmq_url_from_k8s_secret():
    if shutil.which("kubectl") is None:
        log_status("kubectl not found, cannot inspect RabbitMQ secret for credentials")
        return None

    for namespace in RABBITMQ_SECRET_NAMESPACES:
        log_status(
            f"Attempting to read RabbitMQ credentials from secret {RABBITMQ_SECRET_NAME} in namespace {namespace}"
        )
        result = subprocess.run(
            ["kubectl", "-n", namespace, "get", "secret", RABBITMQ_SECRET_NAME, "-o", "json"],
            capture_output=True,
            text=True,
            timeout=10,
            check=False,
        )

        if result.returncode != 0:
            continue

        try:
            secret = json.loads(result.stdout)
            username = base64.b64decode(secret["data"]["username"]).decode("utf-8")
            password = base64.b64decode(secret["data"]["password"]).decode("utf-8")
            secret_url = f"amqp://{username}:{password}@localhost:5672/"
            log_status(f"Recovered RabbitMQ credentials from namespace {namespace}")
            return secret_url
        except Exception as exc:
            log_status(f"Failed to parse RabbitMQ secret in namespace {namespace}: {exc}")

    log_status("Could not read RabbitMQ credentials from any configured Kubernetes namespace")
    return None


def get_rabbitmq_connection_candidates(primary_url):
    candidates = [primary_url]

    if "guest:guest@localhost:5672" in primary_url:
        secret_url = get_rabbitmq_url_from_k8s_secret()
        if secret_url:
            candidates.append(secret_url)

    return dedupe(candidates)


def connect_rabbitmq():
    rabbit_url = get_local_rabbitmq_url()

    for candidate_url in get_rabbitmq_connection_candidates(rabbit_url):
        try:
            log_status(f"Connecting to RabbitMQ at {candidate_url}")
            parameters = pika.URLParameters(candidate_url)
            connection = pika.BlockingConnection(parameters)
            channel = connection.channel()
            channel.queue_declare(queue="pager_messages", durable=True)
            log_status("Connected to RabbitMQ and declared pager_messages queue")
            return connection, channel
        except Exception as exc:
            log_status(f"RabbitMQ connection attempt failed for {candidate_url}: {exc}")

    print(f"Error: Could not connect to RabbitMQ at {rabbit_url}")
    print("Details: all configured and discovered credential attempts failed")
    print("\nHints:")
    print("  1. Ensure RabbitMQ is port-forwarded locally:")
    print("     kubectl -n flux-system port-forward service/firepager-rmq 5672:5672")
    print("  2. If running outside Kubernetes, set RABBITMQ_URL explicitly.")
    print("  3. If port-forward is flaky, use the NodePort service if available.")
    raise SystemExit(1)


def publish_payload(channel, payload):
    channel.basic_publish(
        exchange="",
        routing_key="pager_messages",
        body=json.dumps(payload),
        properties=pika.BasicProperties(delivery_mode=2),
    )


def publish_with_reconnect(conn, channel, payload):
    try:
        publish_payload(channel, payload)
        return conn, channel
    except (AMQPError, StreamLostError, BrokenPipeError, OSError) as exc:
        log_status(f"Publish failed due to connection loss: {exc}")
        log_status("Reconnecting to RabbitMQ and retrying publish once")
        new_conn, new_channel = connect_rabbitmq()
        publish_payload(new_channel, payload)
        log_status("Publish retry succeeded after reconnect")
        return new_conn, new_channel


def send_message(conn, channel, capcode, text):
    payload = {
        "source": capcode,
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "text": text,
    }
    conn, channel = publish_with_reconnect(conn, channel, payload)
    log_status(f"Sent text message for CAPCODE {capcode}: {text}")
    return conn, channel


def send_raw_message(conn, channel, raw_text):
    payload = {
        "client_id": "simulator",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "raw_text": raw_text,
    }
    conn, channel = publish_with_reconnect(conn, channel, payload)
    log_status(f"Sent raw simulated call: {raw_text}")
    return conn, channel


def mutate_example_call(raw_text):
    page_id = "".join(random.choices("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", k=6))
    incident = str(random.randint(30000, 99999))
    timestamp = datetime.now().strftime("%H:%M:%S")

    updated = re.sub(r"ID:[A-Za-z0-9]+", f"ID:{page_id}", raw_text, count=1)
    updated = re.sub(r"#\d{4,6}", f"#{incident}", updated, count=1)
    updated = re.sub(r"\b\d{2}:\d{2}:\d{2}\b", timestamp, updated, count=1)
    return updated


def normalize_raw_call(output):
    cleaned = output.strip().replace("```", "")

    if "POCSAG" in cleaned:
        cleaned = cleaned[cleaned.index("POCSAG") :]

    cleaned = cleaned.replace("\r", "")
    cleaned = cleaned.replace("\n", "<LF>")
    cleaned = re.sub(r"<LF>+", "<LF>", cleaned).strip()

    if not cleaned.endswith("<EOT><NUL>") and "<EOT>" not in cleaned:
        cleaned += "<EOT><NUL>"
    elif cleaned.endswith("<EOT>"):
        cleaned += "<NUL>"

    return cleaned


def ollama_api_request(path, payload, timeout):
    request = urllib.request.Request(
        f"{OLLAMA_HOST}{path}",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        return json.loads(response.read().decode("utf-8"))


def is_ollama_api_ready(timeout):
    try:
        with urllib.request.urlopen(f"{OLLAMA_HOST}/api/tags", timeout=timeout) as response:
            return response.status == 200
    except Exception:
        return False


def ensure_ollama_container_running(timeout):
    if shutil.which("docker") is None:
        raise RuntimeError("docker is not installed and no Ollama endpoint is configured")

    log_status(f"Checking Docker container {OLLAMA_CONTAINER_NAME} for Ollama")
    inspect = subprocess.run(
        ["docker", "inspect", "-f", "{{.State.Running}}", OLLAMA_CONTAINER_NAME],
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )

    if inspect.returncode == 0 and inspect.stdout.strip() == "true":
        log_status(f"Ollama container {OLLAMA_CONTAINER_NAME} is already running")
        return

    if inspect.returncode == 0:
        log_status(f"Starting existing Ollama container {OLLAMA_CONTAINER_NAME}")
        start = subprocess.run(
            ["docker", "start", OLLAMA_CONTAINER_NAME],
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        if start.returncode != 0:
            raise RuntimeError(start.stderr.strip() or "failed to start existing Ollama container")
        log_status(f"Started existing Ollama container {OLLAMA_CONTAINER_NAME}")
        return

    log_status(f"Launching new Ollama container {OLLAMA_CONTAINER_NAME} from {OLLAMA_DOCKER_IMAGE}")
    run = subprocess.run(
        [
            "docker",
            "run",
            "-d",
            "--name",
            OLLAMA_CONTAINER_NAME,
            "-p",
            "11434:11434",
            "-v",
            "firepager-ollama:/root/.ollama",
            OLLAMA_DOCKER_IMAGE,
        ],
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )
    if run.returncode != 0:
        raise RuntimeError(run.stderr.strip() or "failed to launch Ollama container")
    log_status(f"Launched new Ollama container {OLLAMA_CONTAINER_NAME}")


def wait_for_ollama_api(timeout):
    log_status(f"Waiting for Ollama API at {OLLAMA_HOST}")
    deadline = time.time() + timeout
    while time.time() < deadline:
        if is_ollama_api_ready(timeout=5):
            log_status("Ollama API is ready")
            return
        time.sleep(1)
    raise RuntimeError(f"Ollama API at {OLLAMA_HOST} did not become ready within {timeout}s")


def ensure_ollama_model_pulled(model, timeout):
    log_status(f"Pulling Ollama model {model}")
    pull = subprocess.run(
        ["docker", "exec", OLLAMA_CONTAINER_NAME, "ollama", "pull", model],
        capture_output=True,
        text=True,
        timeout=max(timeout, 60),
        check=False,
    )
    if pull.returncode != 0:
        raise RuntimeError(pull.stderr.strip() or f"failed to pull Ollama model {model}")
    log_status(f"Model {model} is available")


def ensure_ollama_available(model, timeout):
    if is_ollama_api_ready(timeout=5):
        log_status(f"Using existing Ollama API at {OLLAMA_HOST}")
        return

    log_status("No reachable Ollama API detected, bootstrapping with Docker")
    ensure_ollama_container_running(timeout)
    wait_for_ollama_api(timeout)
    ensure_ollama_model_pulled(model, timeout)


def generate_ollama_raw_call(model, timeout):
    ensure_ollama_available(model, timeout)
    log_status(f"Generating raw pager call with Ollama model {model}")

    prompt = (
        "Generate exactly one realistic Ottawa-area fire/EMS pager dispatch in raw POCSAG format. "
        "Match the examples closely. Use literal <LF> separators instead of real newlines. "
        "Return only the raw pager line, no explanation, no markdown. "
        "It must start with 'POCSAG512: Address:' and end with '<EOT><NUL>'. "
        "Include a realistic ID, call type, timestamp, address, Dv/Sec, Gd, vehicles, incident number, and Sent by line.\n\n"
        "Examples:\n"
        f"{RAW_CALL_EXAMPLES[0]}\n"
        f"{RAW_CALL_EXAMPLES[1]}\n"
        f"{RAW_CALL_EXAMPLES[2]}\n"
    )

    raw_response = ollama_api_request(
        "/api/generate",
        {"model": model, "prompt": prompt, "stream": False},
        timeout=timeout,
    )

    raw_call = normalize_raw_call(raw_response.get("response", ""))
    if "POCSAG512: Address:" not in raw_call or "Alpha:" not in raw_call:
        raise RuntimeError("ollama returned output that does not match pager-call format")

    log_status("Received Ollama response in pager-call format")
    return raw_call


def close_connection(conn):
    if conn is None:
        return

    try:
        if conn.is_open:
            conn.close()
            log_status("Closed RabbitMQ connection")
        else:
            log_status("RabbitMQ connection was already closed")
    except ConnectionWrongStateError:
        log_status("RabbitMQ connection was already closed")


def main():
    parser = argparse.ArgumentParser(description="FirePager Message Simulator")
    parser.add_argument("--capcode", type=str, help="Specific CAPCODE to use")
    parser.add_argument("--text", type=str, help="Specific message text to send")
    parser.add_argument("--count", type=int, default=1, help="Number of messages to send (0 for infinite)")
    parser.add_argument("--delay", type=float, default=2.0, help="Delay between messages in seconds")
    parser.add_argument("--random", action="store_true", help="Use random fire-themed messages")
    parser.add_argument("--ollama", action="store_true", help="Use Ollama to generate realistic raw pager calls")
    parser.add_argument("--ollama-model", type=str, default="gemma3:1b", help="Ollama model to use for generated calls")
    parser.add_argument("--ollama-timeout", type=int, default=300, help="Ollama generation timeout in seconds")
    args = parser.parse_args()

    log_status(
        f"Starting simulator with count={args.count}, delay={args.delay}, "
        f"random={args.random}, ollama={args.ollama}"
    )

    conn, channel = connect_rabbitmq()

    fire_messages = [
        "STRUCTURE FIRE - 123 MAIN ST - SMOKE SHOWING FROM ROOF",
        "MVC - HWY 401 KM 250 - 2 VEHICLES INVOLVED",
        "MEDICAL EMERGENCY - 456 OAK AVE - 70YOM CHEST PAIN",
        "ALARM - 789 PINE RD - COMMERCIAL SMOKE DETECTOR",
        "BRUSH FIRE - REAR OF NORTH PARK - FAST SPREADING",
    ]

    capcodes = ["0930032", "0910000", "0630000", "0001296"]

    try:
        sent = 0
        while True:
            log_status(f"Preparing message {sent + 1}")
            cap = args.capcode or random.choice(capcodes)

            if args.ollama:
                try:
                    raw_text = generate_ollama_raw_call(args.ollama_model, args.ollama_timeout)
                except Exception as exc:
                    log_status(f"Warning: Ollama call generation failed: {exc}")
                    log_status("Falling back to a mutated example call")
                    raw_text = mutate_example_call(random.choice(RAW_CALL_EXAMPLES))
                conn, channel = send_raw_message(conn, channel, raw_text)
            elif args.text:
                log_status("Using explicit --text payload")
                conn, channel = send_message(conn, channel, cap, args.text)
            elif args.random:
                log_status("Using built-in random fire-themed message")
                text = f"{random.choice(fire_messages)} - {datetime.now().strftime('%H:%M:%S')}"
                conn, channel = send_message(conn, channel, cap, text)
            else:
                log_status("Using default test message template")
                text = f"TEST MESSAGE #{sent + 1} at {datetime.now().strftime('%H:%M:%S')}"
                conn, channel = send_message(conn, channel, cap, text)

            sent += 1
            if args.count > 0 and sent >= args.count:
                log_status(f"Reached requested message count: {sent}")
                break

            log_status(f"Sleeping for {args.delay} seconds before next message")
            time.sleep(args.delay)

    except KeyboardInterrupt:
        log_status("Simulator stopped by user")
    finally:
        log_status("Closing RabbitMQ connection")
        close_connection(conn)


if __name__ == "__main__":
    main()

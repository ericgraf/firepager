# Receiver waterfall snapshots

The Pi RTL-SDR can run as a fixed-gain, spectrum-only source while the three
host SDR decoders continue receiving POCSAG. The Pi continuously scans
155.538–155.658 MHz with `rtl_power` at 5-second integration and publishes the
most recent five-minute window (up to 60 rows) to the durable
`receiver_waterfalls` RabbitMQ queue.

The processor stores snapshots in PostgreSQL for 48 hours. The admin API
`GET /api/admin/receivers/remote-pi-sdr/waterfalls?limit=12` serves recent
snapshots to the Receiver Fleet section of the web UI. The endpoint is
admin-authenticated. The Pi publisher uses the existing RabbitMQ client
credentials, publisher confirms, a 1 MiB message bound, and a shared SDR lock.

This is measured RF power data, not POCSAG decode evidence. The Pi is not a
pager decoder in this role; the host SDRs remain the active decoders. No
frequency calibration or retuning is performed by the spectrum publisher.

The Pi role is managed with the fleet adapter:

```sh
tools/sdr-fleet/adapter.py start_waterfall --receiver-id remote-pi-sdr
tools/sdr-fleet/adapter.py health_check --receiver-id remote-pi-sdr
```

Use `stop_waterfall` before assigning the Pi to a decoder or detector role.

# Receiver status overview

Receiver telemetry uses the durable RabbitMQ queue `receiver_status`, separate
from the `pager_messages` queue. The host-side rotation state publisher emits a
snapshot for each receiver every 20 seconds; the backend processor validates
and upserts snapshots into Postgres. The admin-only `GET /api/admin/receivers`
endpoint returns the latest row per receiver and reports snapshots older than
90 seconds as `stale`. The Admin Dashboard polls this endpoint every 15 seconds.

Snapshots contain receiver identity/host, role and controller status, tuned
frequency, gain and PPM, active detector flag, controller decode/calibration
counters, last accepted calibration time, and observation time. They contain
no pager text or broker credentials. Pager decode events continue unchanged on
their existing queue.

## Rollout

1. Build and deploy the backend API and processor. `db.InitDB()` creates the
   latest-state table; the processor consumes `receiver_status` using its
   existing `RABBITMQ_URL` secret configuration.
2. Install the publisher service/timer from `tools/sdr-fleet/` as user units on
   the rotation-controller host and enable the timer. The publisher obtains
   the existing broker URL from the host environment or in-memory from a
   configured FirePager client container; it never writes or logs credentials.
3. Build/deploy the frontend containing the Receiver Fleet panel.
4. Verify queue consumption in processor logs and view receiver timestamps in
   the authenticated admin dashboard. Stale entries are retained for diagnosis
   and visibly marked stale; they are not treated as online.

The code and units are staged in the repository. Production backend/frontend
deployment and enabling the publisher timer are separate operational steps.

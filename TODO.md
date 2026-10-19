# Firepager v2 — Cloud Deployment TODO

## Target environment (`cloud.graffamily.ca`, new OpenStack cloud)
- Kubernetes v1.36.5 (RKE2), 3 nodes: 1 control-plane + 2 workers
- FluxCD + SOPS (GitOps; `apps` and dependency-ordered Firepager Kustomizations enabled)
- Gateway API / Cilium shared gateway (193.16.110.111), cert-manager + Let's Encrypt
- Cinder CSI storage: `csi-cinder-sc-delete` (default), `csi-cinder-sc-retain`
- Public DNS working: `rancher.cloud.graffamily.ca` is live

## Requirements
- **Multiple SDR clients on Raspberry Pi**, each sending to the server (multi-node ingest, unique `client_id` per Pi)
- Durability goal: retain messages during failures; **RabbitMQ runs as a single replica** (brief downtime is acceptable). Zero loss still requires client buffering and publisher confirmations.

## TODO

### 1. Registry & image pipeline — decided: GHCR
- [x] Public repo `ghcr.io/ericgraf/firepager` created
- [x] GitHub Actions publishes backend, frontend and multi-arch (arm64 + amd64) client images to public GHCR
- [x] Backend/frontend images pinned by digest in GitOps; anonymous pulls verified

### 2. Database (PostgreSQL) — decided: CloudNativePG
- [x] CNPG operator chart 0.29.1 (HelmRelease) + 2-replica `firepager` cluster on `csi-cinder-sc-retain` — committed to flux-cloud
- [x] SOPS-encrypted CNPG basic-auth secret (`username`, `password`) wired into initdb + backend; app uses `firepager-rw`
- [x] Both replicas healthy, storage Bound
- [ ] Configure database-aware off-cluster backups and test restore (replicas/retained PVCs are not backups)

### 3. Message broker (RabbitMQ) — decided: single replica
- [x] Official `rabbitmq:4.1.3` single-replica StatefulSet, retained volume, `rabbitmq-auth` SOPS secret
- [x] Public endpoint: LoadBalancer Service, `external-dns.kubernetes.io/hostname: rabbitmq.cloud.graffamily.ca`
- [x] ExternalDNS publishes `rabbitmq.cloud.graffamily.ca` → 193.16.110.48
- [x] Public AMQPS only (5671), valid Let's Encrypt certificate; plaintext and management not publicly exposed
- [x] Queue `pager_messages` durable; shared application user currently `firepager`
- [ ] Automate broker TLS reload/restart after certificate renewal

### 4. Backend & app stack
- [x] Manifests ported: backend ×2, processor ×1, frontend ×1, sizes for 2-vCPU workers — committed
- [x] Flux deployment enabled; all pods Ready
- [x] Public AMQPS publisher-confirmed test message decoded and returned by authenticated API
- [x] Random SOPS-managed admin bootstrap password replaces development default

### 5. Frontend — deployed
- [x] HTTPRoute `firepager.cloud.graffamily.ca` on shared gateway (`/api` → backend, rest → nginx) — committed
- [x] Shared gateway with independent Firepager certificate; HTTPS returns 200
- [x] ExternalDNS manages Firepager DNS automatically

### 6. SDR clients (Raspberry Pi fleet)
- [x] Multi-arch client image published to GHCR (see source repo Actions)
- [ ] Configure each Pi: `RABBITMQ_URL=amqps://firepager:<pw>@rabbitmq.cloud.graffamily.ca:5671/`, unique `CLIENT_ID`, `PAGER_FREQ`; verify hardware
- [x] Basic reconnect loop exists (5s retries)
- [x] Harden SDR subprocess cleanup, redact credentials from logs, add local spooling and publisher confirmations; tests pass
- [x] Add receive-only RTL-SDR FM-reference calibration with per-device PPM/gain settings and jittered periodic recalibration
- [x] Confirm Ottawa Fire paging allocation at 155.610 MHz from current ISED TAFL data; use as the client default target
- [ ] Verify the 155.610 MHz target with live POCSAG traffic before applying automatic PPM corrections
- [ ] Validate calibration on each Pi with multiple strong local FM stations; tune station list, gain, and outage schedule
- [ ] Fleet visibility: dashboard shows per-Pi last-seen / health (client_id already in schema)
- [ ] Provision least-privilege per-Pi broker users (currently shared application account)

### 7. Verification
- [x] Synthetic Pi message → public AMQPS → processor → Postgres → authenticated API; UI HTTPS healthy
- [ ] Verify rendering with real Pi traffic in browser
- [ ] Kill-a-node test with 2 active Pi clients publishing
- [ ] Failure-mode test: broker down, message durability, no loss

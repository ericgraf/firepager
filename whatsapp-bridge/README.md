# Unofficial WhatsApp bridge (opt-in)

This is an isolated, local Node.js bridge using the Hermes-style Baileys
workflow: persistent multi-file auth, QR pairing, connection health, and an
authenticated send endpoint. **It does not connect or pair by default.** The
default mode is `mock`, and the backend continues to use official Meta Cloud
(or existing Twilio paths) unless `WHATSAPP_PROVIDER=baileys` is explicitly
selected.

## Development / startup

```sh
cd whatsapp-bridge
npm install
BRIDGE_AUTH_TOKEN='use-a-long-random-secret' npm test
BRIDGE_AUTH_TOKEN='use-a-long-random-secret' npm start # mock; no WhatsApp connection
```

To deliberately opt in to a real account, set **all** of these in a protected
environment (never in source control):

```sh
BRIDGE_ENABLED=true BRIDGE_MODE=baileys BRIDGE_AUTH_TOKEN='...' npm start
# backend: WHATSAPP_GATEWAY_ENABLED=true WHATSAPP_PROVIDER=baileys
# WHATSAPP_BRIDGE_URL=http://127.0.0.1:8787 WHATSAPP_BRIDGE_AUTH_TOKEN='...'
```

Open `GET /pairing` with the bearer token to retrieve the QR data URL. Send is
`POST /send` with `{ "to": "+E164", "text": "..." }`. Use
`BRIDGE_ALLOWLIST=+...,+...` to restrict recipients. `/health` is unauthenticated
and contains no QR or secret.

## Security, compliance, and operations

Baileys is unofficial automation. WhatsApp terms, account bans, disconnects,
message loss, and protocol breakage are possible; obtain informed user opt-in,
use a narrow allowlist, and prefer Meta Cloud API/Twilio for production. Do not
use this bridge for unsolicited messaging. Never log QR values, auth files,
tokens, phone numbers, or message bodies. `BRIDGE_AUTH_TOKEN` and the backend
token must be long random secrets, delivered through a secret manager and
rotated together.

The auth directory contains reusable session credentials. In Docker mount a
private volume at `/app/data/auth`; in Kubernetes use an encrypted, access-
restricted PVC with a single bridge replica (or a carefully coordinated
leader). Back it up only encrypted and delete it to force re-pairing. Keep the
bridge reachable only from the backend network; do not expose its port through
an ingress. Rate limiting is enabled on the local send endpoint.

Tests use mock mode and never contact WhatsApp or pair an account.

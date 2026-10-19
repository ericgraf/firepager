/*
 * Local, opt-in WhatsApp bridge. It intentionally does not create a Baileys
 * socket unless BRIDGE_ENABLED=true and MODE=baileys. The default is mock.
 * Keep this process on localhost/a private network; it is not an auth proxy.
 */
const http = require('node:http');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');

const config = {
  host: process.env.BRIDGE_HOST || '127.0.0.1',
  port: Number(process.env.BRIDGE_PORT || 8787),
  enabled: /^(1|true|yes|on)$/i.test(process.env.BRIDGE_ENABLED || 'false'),
  mode: (process.env.BRIDGE_MODE || 'mock').toLowerCase(),
  token: process.env.BRIDGE_AUTH_TOKEN || '',
  authDir: process.env.BRIDGE_AUTH_DIR || path.join(__dirname, 'data', 'auth'),
  allowlist: (process.env.BRIDGE_ALLOWLIST || '').split(',').map(x => x.trim()).filter(Boolean),
  limit: Number(process.env.BRIDGE_RATE_LIMIT || 10),
  windowMs: Number(process.env.BRIDGE_RATE_WINDOW_MS || 60000),
};
const buckets = new Map();
let qr = null;
let connection = 'mock';
let sendImpl = async () => ({ id: `mock-${crypto.randomUUID()}` });

function safe(value) { return value ? '<redacted>' : '<empty>'; }
function json(res, status, body) {
  res.writeHead(status, { 'content-type': 'application/json', 'cache-control': 'no-store' });
  res.end(JSON.stringify(body));
}
function authorized(req) {
  const supplied = req.headers.authorization?.replace(/^Bearer\s+/i, '') || '';
  return Boolean(config.token) && supplied.length === config.token.length &&
    crypto.timingSafeEqual(Buffer.from(supplied), Buffer.from(config.token));
}
function allowed(to) { return config.allowlist.length === 0 || config.allowlist.includes(to); }
function limited(req) {
  const key = req.socket.remoteAddress || 'local';
  const now = Date.now();
  let bucket = buckets.get(key);
  if (!bucket || now - bucket.started >= config.windowMs) bucket = { started: now, count: 0 };
  bucket.count++;
  buckets.set(key, bucket);
  return bucket.count > config.limit;
}
async function body(req) {
  let raw = '';
  for await (const chunk of req) { raw += chunk; if (raw.length > 16384) throw new Error('payload too large'); }
  return JSON.parse(raw || '{}');
}
function safeError() { return { error: 'bridge provider failed' }; }

async function startBaileys() {
  // Dynamic loading keeps mock/test mode independent from Baileys and avoids
  // any WhatsApp connection during development or test execution.
  const baileys = require('@whiskeysockets/baileys');
  const QRCode = require('qrcode');
  fs.mkdirSync(config.authDir, { recursive: true, mode: 0o700 });
  const { state, saveCreds } = await baileys.useMultiFileAuthState(config.authDir);
  const sock = baileys.default({ auth: state, printQRInTerminal: false, browser: ['Firepager', 'Bridge', '0.1'] });
  connection = state.creds.registered ? 'connecting' : 'pairing';
  sock.ev.on('creds.update', saveCreds);
  sock.ev.on('connection.update', async update => {
    if (update.qr) qr = await QRCode.toDataURL(update.qr);
    if (update.connection) connection = update.connection;
    if (update.connection === 'open') qr = null;
  });
  sendImpl = async ({ to, text }) => {
    if (connection !== 'open') throw new Error('not connected');
    const result = await sock.sendMessage(`${to.slice(1)}@s.whatsapp.net`, { text });
    return { id: result?.key?.id || 'sent' };
  };
}

async function handler(req, res) {
  if (req.method === 'GET' && req.url === '/health') {
    return json(res, 200, { ok: true, enabled: config.enabled, mode: config.mode, connection });
  }
  if (!authorized(req)) return json(res, 401, { error: 'unauthorized' });
  if (req.method === 'GET' && req.url === '/pairing') return json(res, 200, { connection, qr });
  if (req.method !== 'POST' || req.url !== '/send') return json(res, 404, { error: 'not found' });
  if (!config.enabled) return json(res, 503, { error: 'bridge disabled' });
  if (limited(req)) return json(res, 429, { error: 'rate limit exceeded' });
  try {
    const payload = await body(req);
    const to = String(payload.to || '');
    const text = String(payload.text || '');
    if (!/^\+[1-9]\d{7,14}$/.test(to) || !text || text.length > 1200 || !allowed(to))
      return json(res, 400, { error: 'invalid or disallowed message' });
    const result = await sendImpl({ to, text });
    return json(res, 200, { ok: true, id: result.id });
  } catch (err) {
    // Never expose Baileys errors, message content, phone numbers, or tokens.
    console.error('bridge send failed', safe(err?.message));
    return json(res, 502, safeError());
  }
}

if (require.main === module) {
  if (config.mode === 'baileys' && config.enabled) startBaileys().catch(err => {
    console.error('bridge provider failed', safe(err.message)); process.exitCode = 1;
  });
  else connection = config.mode === 'mock' ? 'mock' : 'disabled';
  http.createServer(handler).listen(config.port, config.host, () =>
    console.log(`WhatsApp bridge listening on ${config.host}:${config.port} (${config.mode})`));
}

module.exports = { handler, config, buckets, safeError, setSendImpl: fn => { sendImpl = fn; } };

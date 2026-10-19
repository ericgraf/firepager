const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { handler, config, buckets, setSendImpl } = require('./server');

function request(method, url, headers = {}, data) {
  return new Promise((resolve, reject) => {
    const req = http.request(url, { method, headers }, res => {
      let raw = ''; res.on('data', x => raw += x); res.on('end', () => resolve({ status: res.statusCode, body: JSON.parse(raw) }));
    }); req.on('error', reject); if (data) req.end(JSON.stringify(data)); else req.end();
  });
}
test('health is public and send requires auth', async () => {
  const server = http.createServer(handler).listen(0);
  const url = `http://127.0.0.1:${server.address().port}`;
  assert.equal((await request('GET', `${url}/health`)).status, 200);
  assert.equal((await request('POST', `${url}/send`, { 'content-type': 'application/json' }, { to: '+12125550100', text: 'x' })).status, 401);
  server.close();
});
test('rate limits authenticated requests', async () => {
  const old = { token: config.token, enabled: config.enabled, limit: config.limit };
  config.token = 'test-token'; config.enabled = true; config.limit = 1; buckets.clear();
  const server = http.createServer(handler).listen(0); const url = `http://127.0.0.1:${server.address().port}`;
  const headers = { authorization: 'Bearer test-token', 'content-type': 'application/json' };
  assert.equal((await request('POST', `${url}/send`, headers, { to: '+12125550100', text: 'x' })).status, 200);
  assert.equal((await request('POST', `${url}/send`, headers, { to: '+12125550100', text: 'x' })).status, 429);
  server.close(); Object.assign(config, old);
});
test('provider failures are redacted', async () => {
  const old = { token: config.token, enabled: config.enabled, limit: config.limit };
  config.token = 'secret'; config.enabled = true; config.limit = 10; buckets.clear();
  const server = http.createServer(handler).listen(0); const url = `http://127.0.0.1:${server.address().port}`;
  setSendImpl(async () => { throw new Error('token and private message must not escape'); });
  const result = await request('POST', `${url}/send`, { authorization: 'Bearer secret', 'content-type': 'application/json' }, { to: '+12125550100', text: 'private message' });
  assert.equal(result.status, 502); assert.deepEqual(result.body, { error: 'bridge provider failed' });
  server.close(); Object.assign(config, old);
});

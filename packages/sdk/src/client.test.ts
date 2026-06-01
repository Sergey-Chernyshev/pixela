import assert from 'node:assert/strict';
import { test } from 'node:test';

import { PixelaApiError, PixelaClient } from './client';

interface Call {
  url: string;
  init: RequestInit;
}

/** Build a fetch stub that returns scripted responses and records calls. */
function stubFetch(responses: Array<() => Response | Promise<Response>>): {
  fetchImpl: typeof fetch;
  calls: Call[];
} {
  const calls: Call[] = [];
  let i = 0;
  const fetchImpl = (async (url: string | URL | Request, init?: RequestInit) => {
    calls.push({ url: String(url), init: init ?? {} });
    const next = responses[Math.min(i, responses.length - 1)];
    i += 1;
    return next();
  }) as unknown as typeof fetch;
  return { fetchImpl, calls };
}

const json = (status: number, body: unknown): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });

const noSleep = async (): Promise<void> => undefined;

test('createBuild posts with ApiKey auth and returns buildId', async () => {
  const { fetchImpl, calls } = stubFetch([() => json(201, { buildId: 'b1', status: 'RUNNING' })]);
  const client = new PixelaClient({
    baseUrl: 'http://api.test/',
    apiKey: 'secret',
    fetchImpl,
    sleep: noSleep,
  });
  const res = await client.createBuild({
    branch: 'main',
    commitSha: 'abc',
    ciBuildId: 'p1',
    parallelTotal: 1,
  });
  assert.equal(res.buildId, 'b1');
  // Trailing slash on baseUrl is normalized.
  assert.equal(calls[0].url, 'http://api.test/api/v1/builds');
  const headers = calls[0].init.headers as Record<string, string>;
  assert.equal(headers.Authorization, 'ApiKey secret');
});

test('declareSnapshot returns needUpload flag (dedup)', async () => {
  const { fetchImpl } = stubFetch([() => json(200, { snapshotId: 's1', needUpload: false })]);
  const client = new PixelaClient({
    baseUrl: 'http://api.test',
    apiKey: 'k',
    fetchImpl,
    sleep: noSleep,
  });
  const res = await client.declareSnapshot('b1', {
    name: 'home',
    browser: 'chromium',
    viewport: '1280x720',
    imageSha256: 'deadbeef',
    width: 1280,
    height: 720,
    byteSize: 100,
  });
  assert.equal(res.needUpload, false);
});

test('uploadImage PUTs raw bytes with image/png content type', async () => {
  const { fetchImpl, calls } = stubFetch([() => new Response(null, { status: 204 })]);
  const client = new PixelaClient({
    baseUrl: 'http://api.test',
    apiKey: 'k',
    fetchImpl,
    sleep: noSleep,
  });
  await client.uploadImage('deadbeef', Buffer.from([1, 2, 3]));
  assert.equal(calls[0].url, 'http://api.test/api/v1/images/deadbeef');
  assert.equal(calls[0].init.method, 'PUT');
  const headers = calls[0].init.headers as Record<string, string>;
  assert.equal(headers['Content-Type'], 'image/png');
});

test('retries on a transient 503 then succeeds', async () => {
  const { fetchImpl, calls } = stubFetch([
    () => json(503, { error: { code: 'UNAVAILABLE', message: 'down' } }),
    () => json(201, { buildId: 'b2', status: 'RUNNING' }),
  ]);
  const client = new PixelaClient({
    baseUrl: 'http://api.test',
    apiKey: 'k',
    fetchImpl,
    sleep: noSleep,
    maxAttempts: 3,
  });
  const res = await client.createBuild({
    branch: 'm',
    commitSha: 'c',
    ciBuildId: 'p',
    parallelTotal: 1,
  });
  assert.equal(res.buildId, 'b2');
  assert.equal(calls.length, 2);
});

test('does not retry a 4xx and surfaces the error code', async () => {
  const { fetchImpl, calls } = stubFetch([
    () => json(401, { error: { code: 'UNAUTHORIZED', message: 'bad key' } }),
  ]);
  const client = new PixelaClient({
    baseUrl: 'http://api.test',
    apiKey: 'k',
    fetchImpl,
    sleep: noSleep,
    maxAttempts: 3,
  });
  await assert.rejects(
    client.finalizeBuild('b1'),
    (err: unknown) =>
      err instanceof PixelaApiError && err.code === 'UNAUTHORIZED' && err.status === 401,
  );
  assert.equal(calls.length, 1); // no retry on 4xx
});

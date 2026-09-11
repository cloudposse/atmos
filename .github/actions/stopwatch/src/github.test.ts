import assert from "node:assert/strict";
import test from "node:test";

import { GitHubClient } from "./github";

/** Creates a test client that returns responses in order without real delays. */
function clientFor(responses: Response[], delays: number[]): { client: GitHubClient; calls: RequestInit[] } {
  const calls: RequestInit[] = [];
  const testFetch = (async (_input: string | URL | Request, init?: RequestInit): Promise<Response> => {
    calls.push(init ?? {});
    const response = responses.shift();
    assert.ok(response, "test provided enough responses");
    return response;
  }) as typeof fetch;

  return {
    client: new GitHubClient("test-token", {
      baseUrl: "https://example.test",
      fetch: testFetch,
      sleep: async (milliseconds) => {
        delays.push(milliseconds);
      },
    }),
    calls,
  };
}

test("GET retries transient server errors and applies a timeout", async () => {
  const delays: number[] = [];
  const { client, calls } = clientFor([
    new Response("temporary", { status: 502 }),
    Response.json({ ok: true }),
  ], delays);

  assert.deepEqual(await client.request("GET", "/resource"), { ok: true });
  assert.equal(calls.length, 2);
  assert.ok(calls.every((call) => call.signal instanceof AbortSignal));
  assert.deepEqual(delays, [1_000]);
});

test("GET honors rate-limit retry headers", async () => {
  const delays: number[] = [];
  const { client, calls } = clientFor([
    new Response("limited", { status: 403, headers: { "retry-after": "0" } }),
    Response.json({ ok: true }),
  ], delays);

  await client.request("GET", "/rate-limited");
  assert.equal(calls.length, 2);
  assert.deepEqual(delays, [0]);
});

test("GET retries 429 responses", async () => {
  const delays: number[] = [];
  const { client, calls } = clientFor([
    new Response("limited", { status: 429, headers: { "retry-after": "0" } }),
    Response.json({ ok: true }),
  ], delays);

  await client.request("GET", "/too-many-requests");
  assert.equal(calls.length, 2);
  assert.deepEqual(delays, [0]);
});

test("GET preserves the final GitHub error after retries", async () => {
  const delays: number[] = [];
  const { client, calls } = clientFor([
    new Response("first", { status: 503 }),
    new Response("second", { status: 503 }),
    new Response("final response", { status: 503 }),
  ], delays);

  await assert.rejects(
    client.request("GET", "/unavailable"),
    /GitHub API GET \/unavailable returned 503: final response/,
  );
  assert.equal(calls.length, 3);
  assert.deepEqual(delays, [1_000, 2_000]);
});

test("mutating requests are never retried", async () => {
  const delays: number[] = [];
  const { client, calls } = clientFor([
    new Response("temporary", { status: 502 }),
    Response.json({ unexpected: true }),
  ], delays);

  await assert.rejects(
    client.request("POST", "/comments", { body: "hello" }),
    /GitHub API POST \/comments returned 502: temporary/,
  );
  assert.equal(calls.length, 1);
  assert.deepEqual(delays, []);
});

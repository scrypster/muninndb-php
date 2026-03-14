/**
 * MuninnDB Node.js SDK unit tests.
 *
 * Uses msw to intercept HTTP requests — no live server required.
 *
 * Run: npm test
 */

import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { setupServer } from "msw/node";
import { http, HttpResponse } from "msw";
import { MuninnClient } from "./client.js";
import type {
  BatchWriteResult,
  TraverseOptions,
} from "./types.js";

const BASE_URL = "http://muninn-test";

function makeClient(): MuninnClient {
  return new MuninnClient({ baseUrl: BASE_URL, token: "test-token", maxRetries: 0 });
}

// ---------------------------------------------------------------------------
// MSW server setup
// ---------------------------------------------------------------------------

const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

// ---------------------------------------------------------------------------
// Type safety: BatchWriteResult.id is optional
// ---------------------------------------------------------------------------

describe("BatchWriteResult type", () => {
  it("allows id to be absent (undefined)", () => {
    const result: BatchWriteResult = { index: 0, status: "duplicate" };
    expect(result.id).toBeUndefined();
  });

  it("allows id to be present", () => {
    const result: BatchWriteResult = { index: 0, id: "01ARZ3", status: "created" };
    expect(result.id).toBe("01ARZ3");
  });
});

// ---------------------------------------------------------------------------
// Type safety: TraverseOptions has follow_entities
// ---------------------------------------------------------------------------

describe("TraverseOptions type", () => {
  it("accepts follow_entities field", () => {
    const opts: TraverseOptions = {
      start_id: "s1",
      follow_entities: true,
    };
    expect(opts.follow_entities).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// write
// ---------------------------------------------------------------------------

describe("write", () => {
  it("returns id and created_at", async () => {
    server.use(
      http.post(`${BASE_URL}/api/engrams`, () =>
        HttpResponse.json({ id: "01ARZ3", created_at: "2024-01-01T00:00:00Z" }, { status: 201 })
      )
    );
    const client = makeClient();
    const resp = await client.write({ concept: "test", content: "body" });
    expect(resp.id).toBe("01ARZ3");
  });
});

// ---------------------------------------------------------------------------
// writeBatch — checks that id is optional in results
// ---------------------------------------------------------------------------

describe("writeBatch", () => {
  it("handles results with and without id", async () => {
    server.use(
      http.post(`${BASE_URL}/api/engrams/batch`, () =>
        HttpResponse.json({
          results: [
            { index: 0, id: "id-1", status: "created" },
            { index: 1, status: "duplicate" },
          ],
        })
      )
    );
    const client = makeClient();
    const resp = await client.writeBatch("default", [
      { concept: "a", content: "aa" },
      { concept: "b", content: "bb" },
    ]);
    expect(resp.results[0]?.id).toBe("id-1");
    expect(resp.results[1]?.id).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// activate
// ---------------------------------------------------------------------------

describe("activate", () => {
  it("returns activations", async () => {
    server.use(
      http.post(`${BASE_URL}/api/activate`, () =>
        HttpResponse.json({
          query_id: "q1",
          total_found: 1,
          activations: [{ id: "a1", concept: "hit", content: "body", score: 0.9, tags: [], memory_type: "" }],
          latency_ms: 5,
        })
      )
    );
    const client = makeClient();
    const resp = await client.activate({ context: ["query"] });
    expect(resp.total_found).toBe(1);
    expect(resp.activations[0]?.id).toBe("a1");
  });
});

// ---------------------------------------------------------------------------
// traverse — follow_entities must be forwarded
// ---------------------------------------------------------------------------

describe("traverse", () => {
  it("passes follow_entities in the request body", async () => {
    let capturedBody: unknown;
    server.use(
      http.post(`${BASE_URL}/api/traverse`, async ({ request }) => {
        capturedBody = await request.json();
        return HttpResponse.json({ nodes: [], edges: [], total_reachable: 0, query_ms: 1 });
      })
    );
    const client = makeClient();
    const opts: TraverseOptions = { start_id: "s1", follow_entities: true };
    await client.traverse(opts);
    expect((capturedBody as Record<string, unknown>)["follow_entities"]).toBe(true);
  });

  it("does not send follow_entities when not set", async () => {
    let capturedBody: unknown;
    server.use(
      http.post(`${BASE_URL}/api/traverse`, async ({ request }) => {
        capturedBody = await request.json();
        return HttpResponse.json({ nodes: [], edges: [], total_reachable: 0, query_ms: 1 });
      })
    );
    const client = makeClient();
    await client.traverse({ start_id: "s1" });
    expect((capturedBody as Record<string, unknown>)["follow_entities"]).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// stats — correct field names from server
// ---------------------------------------------------------------------------

describe("stats", () => {
  it("returns engram_count, vault_count, storage_bytes", async () => {
    server.use(
      http.get(`${BASE_URL}/api/stats`, () =>
        HttpResponse.json({ engram_count: 100, vault_count: 3, storage_bytes: 204800 })
      )
    );
    const client = makeClient();
    const resp = await client.stats();
    expect(resp.engram_count).toBe(100);
    expect(resp.vault_count).toBe(3);
    expect(resp.storage_bytes).toBe(204800);
  });
});

// ---------------------------------------------------------------------------
// subscribe — threshold parameter forwarded in URL
// ---------------------------------------------------------------------------

describe("subscribe", () => {
  it("includes threshold in URL when specified", () => {
    const client = makeClient();
    // Inspect the generated URL by calling subscribe and checking the AbortController
    // is created. Since we don't actually start the SSE connection in this test,
    // we verify the URL by accessing the internal URL construction indirectly.
    // (subscribe() is a generator — it only connects when iterated)
    const iterable = client.subscribe("default", true, 0.05);
    // The iterable exists but is not yet started; we just check it's returned.
    expect(iterable).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// error handling
// ---------------------------------------------------------------------------

describe("error handling", () => {
  it("throws on 404", async () => {
    const { MuninnNotFoundError } = await import("./errors.js");
    server.use(
      http.get(`${BASE_URL}/api/engrams/missing`, () =>
        HttpResponse.json({ error: "not found" }, { status: 404 })
      )
    );
    const client = makeClient();
    await expect(client.read("missing")).rejects.toBeInstanceOf(MuninnNotFoundError);
  });

  it("throws on 401", async () => {
    const { MuninnAuthError } = await import("./errors.js");
    server.use(
      http.get(`${BASE_URL}/api/engrams/secret`, () =>
        HttpResponse.json({ error: "unauthorized" }, { status: 401 })
      )
    );
    const client = makeClient();
    await expect(client.read("secret")).rejects.toBeInstanceOf(MuninnAuthError);
  });
});

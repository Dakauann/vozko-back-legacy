import { afterAll, describe, expect, test } from "bun:test";
import type { JobKind, JobReply } from "../src/jobs";
import { JobTimeout, Pool, PoolFull } from "../src/pool";
import { bearerMatches, createHandler, maxBodyBytes, readToken, TokenUnavailable, type Runner } from "../src/server";
import { htmlProject, runtime, shell } from "./fixtures/projects";

const token = "t".repeat(40);

function runner(run: (kind: JobKind, raw: string) => Promise<JobReply>): Runner {
  return { run, queued: 0, busy: 0 };
}

const echo = runner(async (kind, raw) => ({ status: 200, body: JSON.stringify({ kind, raw }) }));

function call(handler: (r: Request) => Promise<Response>, path: string, init: RequestInit & { auth?: string | null } = {}) {
  const headers = new Headers(init.headers);
  if (init.auth !== null) headers.set("authorization", init.auth ?? `Bearer ${token}`);
  return handler(new Request(`http://builder${path}`, { ...init, headers }));
}

describe("handler", () => {
  const handler = createHandler({ token, runner: echo, vendorVersion: "v1" });
  const json = { "content-type": "application/json" };

  test("refuses short tokens at construction", () => {
    expect(() => createHandler({ token: "short", runner: echo, vendorVersion: "v1" })).toThrow();
  });

  test("every request needs the bearer token", async () => {
    for (const auth of [null, "Bearer wrong", `Basic ${token}`, "Bearer "]) {
      const response = await call(handler, "/health", { auth });
      expect(response.status).toBe(401);
      expect(await response.json()).toEqual({ code: "unauthorized", message: "a valid bearer token is required" });
    }
    expect(bearerMatches(`Bearer ${token}`, token)).toBe(true);
  });

  test("health reports the vendor version", async () => {
    const response = await call(handler, "/health");
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ status: "ok", vendor: "v1", busy: 0, queued: 0 });
  });

  test("routes, methods and media types", async () => {
    expect((await call(handler, "/nada", { method: "POST", headers: json, body: "{}" })).status).toBe(404);
    expect((await call(handler, "/build")).status).toBe(405);
    expect((await call(handler, "/build", { method: "POST", body: "{}" })).status).toBe(415);
    const ok = await call(handler, "/execute", { method: "POST", headers: json, body: '{"a":1}' });
    expect(await ok.json()).toEqual({ kind: "execute", raw: '{"a":1}' });
  });

  test("bodies above the limit are refused", async () => {
    const response = await call(handler, "/build", { method: "POST", headers: { ...json, "content-length": String(maxBodyBytes + 1) }, body: "{}" });
    expect(response.status).toBe(413);
  });

  test("busy and timeout map to 503 and 504", async () => {
    const full = createHandler({ token, runner: runner(async () => { throw new PoolFull("full"); }), vendorVersion: "v1" });
    expect((await call(full, "/build", { method: "POST", headers: json, body: "{}" })).status).toBe(503);
    const slow = createHandler({ token, runner: runner(async () => { throw new JobTimeout("slow"); }), vendorVersion: "v1" });
    expect((await call(slow, "/build", { method: "POST", headers: json, body: "{}" })).status).toBe(504);
  });
});

describe("token file", () => {
  const files: Record<string, string> = { "/run/secrets/builder": `${token}\n`, "/run/secrets/short": "short\n", "/run/secrets/blank": "\n" };
  const read = (path: string) => {
    if (files[path] === undefined) throw new Error(`ENOENT: ${path}`);
    return files[path];
  };

  test("reads the token once from the file named by the environment", () => {
    expect(readToken({ SITES_BUILDER_TOKEN_FILE: "/run/secrets/builder" }, read)).toBe(token);
  });

  test("refuses a missing, unreadable, short or blank file", () => {
    for (const env of [{}, { SITES_BUILDER_TOKEN_FILE: "" }, { SITES_BUILDER_TOKEN_FILE: "/run/secrets/none" }, { SITES_BUILDER_TOKEN_FILE: "/run/secrets/short" }, { SITES_BUILDER_TOKEN_FILE: "/run/secrets/blank" }]) {
      expect(() => readToken(env, read)).toThrow(TokenUnavailable);
    }
  });

  test("refuses to start while the token sits in the environment", () => {
    expect(() => readToken({ SITES_BUILDER_TOKEN_FILE: "/run/secrets/builder", SITES_BUILDER_TOKEN: token }, read)).toThrow(TokenUnavailable);
  });

  test("the refusal never echoes the token or the file content", () => {
    try {
      readToken({ SITES_BUILDER_TOKEN_FILE: "/run/secrets/short" }, read);
    } catch (error) {
      expect((error as Error).message).not.toContain("short\n");
      expect((error as Error).message).not.toContain(token);
    }
  });
});

describe("pool", () => {
  const spawn = () => new Worker(new URL("./fixtures/hang-worker.ts", import.meta.url).href);

  test("runs jobs, refuses when the queue is full and survives a timeout", async () => {
    const pool = new Pool({ size: 1, queueLimit: 1, timeouts: { execute: 300, build: 300, extract: 300 }, spawn });
    try {
      const hung = pool.run("build", "hang");
      const queued = pool.run("build", "after");
      await expect(pool.run("build", "third")).rejects.toBeInstanceOf(PoolFull);
      await expect(hung).rejects.toBeInstanceOf(JobTimeout);
      expect(await queued).toEqual({ status: 200, body: "after" });
      expect(await pool.run("execute", "again")).toEqual({ status: 200, body: "again" });
    } finally {
      pool.close();
    }
  });
});

describe("service end to end", () => {
  const pool = new Pool({ size: 2, queueLimit: 4, timeouts: { execute: 20_000, build: 60_000, extract: 20_000 } });
  const server = Bun.serve({ port: 0, fetch: createHandler({ token, runner: pool, vendorVersion: "v1" }) });
  const base = `http://127.0.0.1:${server.port}`;
  const post = (path: string, body: unknown) =>
    fetch(base + path, { method: "POST", headers: { authorization: `Bearer ${token}`, "content-type": "application/json" }, body: JSON.stringify(body) });

  afterAll(() => {
    server.stop(true);
    pool.close();
  });

  test("execute applies a batch in a worker", async () => {
    const response = await post("/execute", {
      project: htmlProject,
      operations: [{ op: "set_text", node: `pages/obrigado.html#0`, rev: 1, text: "Valeu" }],
      texts: { "0": [{ text: "Valeu" }] },
    });
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.steps[0].files["pages/obrigado.html"]).toBe('<h1 class="text-3xl">Valeu</h1>\n');
  });

  test("malformed requests are refused with request_invalid", async () => {
    const response = await post("/execute", { project: htmlProject, operations: [{ op: "paint" }] });
    expect(response.status).toBe(400);
    expect((await response.json()).code).toBe("request_invalid");
    const unknown = await post("/build", { mode: "preview", project: htmlProject, shell, runtime, extra: 1 });
    expect(unknown.status).toBe(400);
  });

  test("build returns base64 assets", async () => {
    const response = await post("/build", { mode: "publish", project: htmlProject, shell, runtime });
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.errors).toEqual([]);
    const index = body.assets.find((asset: { path: string }) => asset.path === "index.html");
    expect(index.content_type).toBe("text/html; charset=utf-8");
    expect(Buffer.from(index.body, "base64").toString("utf8")).toContain("<!DOCTYPE html>");
    expect(Object.keys(body).sort()).toEqual(["assets", "errors", "imports", "prerender_path", "routes", "warnings"]);
  });
});

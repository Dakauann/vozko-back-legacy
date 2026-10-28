import { createHash, timingSafeEqual } from "node:crypto";
import { errorReply, type JobKind, type JobReply } from "./jobs";
import { JobTimeout, PoolFull } from "./pool";

export const maxBodyBytes = 32 * 1024 * 1024;
export const minTokenLength = 32;

export const tokenFileVariable = "SITES_BUILDER_TOKEN_FILE";
export const tokenVariable = "SITES_BUILDER_TOKEN";

export class TokenUnavailable extends Error {}

export function readToken(env: Record<string, string | undefined>, read: (path: string) => string): string {
  if (env[tokenVariable] !== undefined) throw new TokenUnavailable(`${tokenVariable} must not be set; give the token through the file named by ${tokenFileVariable}`);
  const path = env[tokenFileVariable] ?? "";
  if (path === "") throw new TokenUnavailable(`${tokenFileVariable} must name the token file`);
  let content: string;
  try {
    content = read(path);
  } catch {
    throw new TokenUnavailable(`the token file named by ${tokenFileVariable} cannot be read`);
  }
  const token = content.replace(/[\r\n]+$/, "");
  if (token.length < minTokenLength || /\s/.test(token)) throw new TokenUnavailable(`the token file must hold one token of at least ${minTokenLength} characters`);
  return token;
}

export interface Runner {
  run(kind: JobKind, raw: string): Promise<JobReply>;
  readonly queued: number;
  readonly busy: number;
}

export interface ServerOptions {
  token: string;
  runner: Runner;
  vendorVersion: string;
}

function digest(value: string): Buffer {
  return createHash("sha256").update(value).digest();
}

export function bearerMatches(header: string | null, token: string): boolean {
  const presented = header?.startsWith("Bearer ") ? header.slice("Bearer ".length) : "";
  return timingSafeEqual(digest(presented), digest(token)) && presented.length > 0;
}

function respond(reply: JobReply): Response {
  return new Response(reply.body, {
    status: reply.status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store", "x-content-type-options": "nosniff" },
  });
}

async function readBody(request: Request): Promise<string | null> {
  const declared = Number(request.headers.get("content-length") ?? "0");
  if (declared > maxBodyBytes) return null;
  if (!request.body) return "";
  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > maxBodyBytes) {
      await reader.cancel();
      return null;
    }
    chunks.push(value);
  }
  return new TextDecoder("utf-8", { fatal: true }).decode(Buffer.concat(chunks));
}

const routes: Record<string, JobKind> = { "/execute": "execute", "/build": "build", "/extract": "extract" };

export function createHandler(options: ServerOptions): (request: Request) => Promise<Response> {
  if (options.token.length < minTokenLength) throw new Error(`SITES_BUILDER_TOKEN must have at least ${minTokenLength} characters`);
  return async (request: Request) => {
    if (!bearerMatches(request.headers.get("authorization"), options.token)) return respond(errorReply(401, "unauthorized", "a valid bearer token is required"));
    const path = new URL(request.url).pathname;
    if (path === "/health") {
      if (request.method !== "GET") return respond(errorReply(405, "method_not_allowed", "use GET"));
      return respond({ status: 200, body: JSON.stringify({ status: "ok", vendor: options.vendorVersion, busy: options.runner.busy, queued: options.runner.queued }) });
    }
    const kind = routes[path];
    if (!kind) return respond(errorReply(404, "not_found", "unknown path"));
    if (request.method !== "POST") return respond(errorReply(405, "method_not_allowed", "use POST"));
    if (!(request.headers.get("content-type") ?? "").toLowerCase().startsWith("application/json")) {
      return respond(errorReply(415, "unsupported_media_type", "send application/json"));
    }
    let raw: string | null;
    try {
      raw = await readBody(request);
    } catch {
      return respond(errorReply(400, "request_invalid", "the body is not valid UTF-8"));
    }
    if (raw === null) return respond(errorReply(413, "request_too_large", `the body exceeds ${maxBodyBytes} bytes`));
    try {
      return respond(await options.runner.run(kind, raw));
    } catch (error) {
      if (error instanceof PoolFull) return respond(errorReply(503, "busy", error.message));
      if (error instanceof JobTimeout) return respond(errorReply(504, "timeout", error.message));
      console.error("sitebuilder job failed", error);
      return respond(errorReply(500, "internal", "the builder failed"));
    }
  };
}

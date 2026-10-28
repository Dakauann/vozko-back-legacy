import { build, BuildRefused } from "./build";
import { execute, extract } from "./execute/executor";
import { decodeBuild, decodeExecute, decodeExtract, RequestInvalid } from "./protocol";

export type JobKind = "execute" | "build" | "extract";

export interface JobReply {
  status: number;
  body: string;
}

export function errorReply(status: number, code: string, message: string): JobReply {
  return { status, body: JSON.stringify({ code, message }) };
}

function base64(bytes: Uint8Array): string {
  return Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength).toString("base64");
}

export async function runJob(kind: JobKind, raw: string): Promise<JobReply> {
  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    return errorReply(400, "request_invalid", "the body is not valid JSON");
  }
  try {
    if (kind === "execute") {
      const result = await execute(decodeExecute(payload));
      return { status: 200, body: JSON.stringify(result) };
    }
    if (kind === "extract") return { status: 200, body: JSON.stringify(extract(decodeExtract(payload))) };
    const result = await build(decodeBuild(payload));
    return {
      status: 200,
      body: JSON.stringify({ ...result, assets: result.assets.map((asset) => ({ path: asset.path, content_type: asset.content_type, body: base64(asset.body) })) }),
    };
  } catch (error) {
    if (error instanceof RequestInvalid) return errorReply(400, "request_invalid", error.message);
    if (error instanceof BuildRefused) return errorReply(422, "build_refused", error.message);
    console.error(`sitebuilder ${kind} failed`, error);
    return errorReply(500, "internal", "the builder failed");
  }
}

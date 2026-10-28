import { availableParallelism } from "node:os";
import { readFileSync } from "node:fs";
import { loadVendor } from "./build/vendor";
import { Pool } from "./pool";
import { createHandler, maxBodyBytes, readToken, TokenUnavailable } from "./server";

const port = 8080;
const queueLimit = 64;
const workers = Math.max(1, Math.min(4, availableParallelism()));
const timeouts = { execute: 10_000, build: 30_000, extract: 10_000 };

let token: string;
try {
  token = readToken(process.env, (path) => readFileSync(path, "utf8"));
} catch (error) {
  if (!(error instanceof TokenUnavailable)) throw error;
  console.error(error.message);
  process.exit(1);
}

const vendor = loadVendor();
const pool = new Pool({ size: workers, queueLimit, timeouts });
const handler = createHandler({ token, runner: pool, vendorVersion: vendor.manifest.version });

const server = Bun.serve({ port, hostname: "0.0.0.0", maxRequestBodySize: maxBodyBytes + 1024, fetch: handler });
console.log(`sitebuilder listening on ${server.port} with ${workers} workers`);

for (const signal of ["SIGINT", "SIGTERM"] as const) {
  process.on(signal, () => {
    server.stop();
    pool.close();
    process.exit(0);
  });
}

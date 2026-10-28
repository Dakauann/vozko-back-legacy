import { existsSync } from "node:fs";
import { join } from "node:path";
import { vendorDir } from "../src/build/vendor";

if (!existsSync(join(vendorDir, "manifest.json"))) {
  const built = Bun.spawnSync(["bun", "run", join(vendorDir, "..", "scripts", "build-vendor.ts")], { stdout: "inherit", stderr: "inherit" });
  if (built.exitCode !== 0) throw new Error("the vendor build failed");
}

import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import type { Project } from "../../src/project";

const startersRoot = fileURLToPath(new URL("../../../../usecases/site/starters/", import.meta.url));

export function filesUnder(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? filesUnder(path) : [path];
  });
}

export function starters(): Array<{ name: string; project: Project }> {
  if (!existsSync(startersRoot)) return [];
  const out: Array<{ name: string; project: Project }> = [];
  for (const stack of ["react", "html"] as const) {
    const stackDir = join(startersRoot, stack);
    if (!existsSync(stackDir)) continue;
    for (const key of readdirSync(stackDir)) {
      const root = join(stackDir, key, "files");
      if (!existsSync(root)) continue;
      const files: Project["files"] = {};
      for (const path of filesUnder(root)) files[relative(root, path).replaceAll("\\", "/")] = { content: readFileSync(path, "utf8"), rev: 1 };
      out.push({ name: `${stack}/${key}`, project: { stack, files } });
    }
  }
  return out;
}

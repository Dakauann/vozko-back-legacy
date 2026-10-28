import * as esbuild from "esbuild";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const outDir = join(root, "vendor");
const entries = JSON.parse(readFileSync(join(root, "src", "vendor-entries.json"), "utf8")) as {
  modules: Record<string, string[]>;
  aliases: Record<string, string>;
  styles: Record<string, string[]>;
  inline: string[];
  html: string[];
};

const identifier = /^[A-Za-z_$][\w$]*$/;
const virtualPrefix = "vz-vendor:";

async function staticExports(code: string): Promise<{ names: string[]; ok: boolean }> {
  try {
    const result = await esbuild.build({
      stdin: { contents: code, resolveDir: root, loader: "js" },
      bundle: true,
      write: false,
      format: "esm",
      platform: "browser",
      metafile: true,
      logLevel: "silent",
      outdir: "out",
    });
    const output = Object.values(result.metafile!.outputs).find((o) => o.entryPoint);
    return { names: output?.exports ?? [], ok: true };
  } catch {
    return { names: [], ok: false };
  }
}

async function runtimeExports(spec: string): Promise<string[]> {
  try {
    const mod = await import(spec);
    return Object.keys(mod);
  } catch {
    return [];
  }
}

async function wrapper(spec: string): Promise<string> {
  const star = await staticExports(`export * from ${JSON.stringify(spec)};`);
  const names = new Set(star.names);
  if (names.size === 0) for (const name of await runtimeExports(spec)) names.add(name);
  names.delete("default");
  names.delete("__esModule");
  const exported = [...names].filter((name) => identifier.test(name)).sort();
  const hasDefault = (await staticExports(`export { default } from ${JSON.stringify(spec)};`)).ok;
  const lines: string[] = [];
  if (exported.length > 0) lines.push(`export { ${exported.join(", ")} } from ${JSON.stringify(spec)};`);
  if (hasDefault) lines.push(`export { default } from ${JSON.stringify(spec)};`);
  if (lines.length === 0) throw new Error(`${spec} exports nothing`);
  return lines.join("\n");
}

function entryName(spec: string): string {
  return spec.replace(/^@/, "").replace(/\.js$/, "").replace(/[^A-Za-z0-9]+/g, "-");
}

async function main() {
  rmSync(outDir, { recursive: true, force: true });
  mkdirSync(outDir, { recursive: true });
  const specs = Object.values(entries.modules).flat();
  const wrappers = new Map<string, string>();
  for (const spec of specs) wrappers.set(spec, await wrapper(spec));
  const moduleBuild = await esbuild.build({
    entryPoints: specs.map((spec) => ({ in: virtualPrefix + spec, out: entryName(spec) })),
    bundle: true,
    splitting: true,
    format: "esm",
    platform: "browser",
    target: "es2020",
    minify: true,
    legalComments: "none",
    write: false,
    metafile: true,
    outdir: outDir,
    entryNames: "[name]-[hash]",
    chunkNames: "chunk-[hash]",
    define: { "process.env.NODE_ENV": '"production"' },
    logLevel: "warning",
    plugins: [
      {
        name: "vendor-wrappers",
        setup(build) {
          build.onResolve({ filter: /^vz-vendor:/ }, (args) => ({ path: args.path.slice(virtualPrefix.length), namespace: "vz-vendor" }));
          build.onLoad({ filter: /.*/, namespace: "vz-vendor" }, (args) => ({ contents: wrappers.get(args.path)!, resolveDir: root, loader: "js" }));
        },
      },
    ],
  });
  const outputs = moduleBuild.metafile!.outputs;
  const fileOf = (path: string) => path.replaceAll("\\", "/").split("/").pop()!;
  for (const file of moduleBuild.outputFiles) writeFileSync(join(outDir, fileOf(file.path)), file.contents);
  const keyOf = (path: string) => Object.keys(outputs).find((key) => fileOf(key) === fileOf(path))!;
  const closure = (key: string, seen = new Set<string>()): string[] => {
    for (const imported of outputs[key].imports) {
      if (imported.kind !== "import-statement" || imported.external) continue;
      const next = keyOf(imported.path);
      if (!seen.has(fileOf(next))) {
        seen.add(fileOf(next));
        closure(next, seen);
      }
    }
    return [...seen].sort();
  };
  const modules: Record<string, { file: string; preload: string[] }> = {};
  for (const [key, output] of Object.entries(outputs)) {
    if (!output.entryPoint) continue;
    const spec = output.entryPoint.replace(/^vz-vendor:/, "");
    modules[spec] = { file: fileOf(key), preload: closure(key) };
  }
  for (const [alias, target] of Object.entries(entries.aliases)) modules[alias] = modules[target];
  const styles: Record<string, string> = {};
  for (const spec of Object.values(entries.styles).flat()) {
    const built = await esbuild.build({
      entryPoints: [spec],
      bundle: true,
      minify: true,
      write: false,
      outdir: outDir,
      absWorkingDir: root,
      loader: { ".css": "css" },
      logLevel: "warning",
    });
    const css = built.outputFiles.find((file) => file.path.endsWith(".css"))!;
    const hash = createHash("sha256").update(css.contents).digest("hex").slice(0, 10);
    const file = `${entryName(spec)}-${hash}.css`;
    writeFileSync(join(outDir, file), css.contents);
    styles[spec] = file;
  }
  const digest = createHash("sha256");
  for (const spec of Object.keys(modules).sort()) digest.update(spec).update(modules[spec].file);
  for (const spec of Object.keys(styles).sort()) digest.update(spec).update(styles[spec]);
  writeFileSync(join(outDir, "manifest.json"), JSON.stringify({ version: digest.digest("hex").slice(0, 16), modules, styles, inline: entries.inline, html: entries.html }, null, 2));
  console.log(`vendor: ${Object.keys(modules).length} modules, ${Object.keys(styles).length} styles`);
}

await main();

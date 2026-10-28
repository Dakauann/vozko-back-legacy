import * as esbuild from "esbuild";
import { mkdtempSync, realpathSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, posix, sep } from "node:path";
import { fileURLToPath } from "node:url";
import type { Stack } from "../project";
import { importDynamicForbidden, importForbidden, sourcePolicy } from "./analyze";
import { tagJsx } from "./tag";
import type { Vendor } from "./vendor";

export interface Import {
  importer: string;
  specifier: string;
}

export interface BuildError {
  path: string;
  line: number;
  column: number;
  message: string;
  code?: string;
}

export interface BundleEntry {
  name: string;
  code?: string;
  path?: string;
}

export interface BundleInput {
  stack: Stack;
  files: Record<string, string>;
  entries: BundleEntry[];
  preview: boolean;
  minify: boolean;
  vendor: Vendor;
  captureRoot?: boolean;
}

export interface OutputFile {
  path: string;
  contents: Uint8Array;
}

export interface EntryOutput {
  file: string;
  chunks: string[];
  vendorModules: string[];
}

export interface BundleOutput {
  files: OutputFile[];
  entries: Record<string, EntryOutput>;
  imports: Import[];
  cssImports: string[];
  vendorStyles: string[];
  graph: Map<string, Set<string>>;
  errors: BuildError[];
}

export const builderRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const projectPrefix = "vz-project:";
const entryPrefix = "vz-entry:";
export const captureSpecifier = "vz-capture:client";
const captureModule = [
  "let captured = null;",
  "const root = { render(element) { captured = element; }, unmount() {} };",
  "export function createRoot() { return root; }",
  "export function hydrateRoot(container, element) { captured = element; return root; }",
  "export function capturedElement() { return captured; }",
].join("\n");

export function projectSpecifier(path: string): string {
  return projectPrefix + path;
}

const moduleExtensions = [".jsx", ".tsx", ".js", ".ts"];

function loaderFor(stack: Stack, path: string): esbuild.Loader {
  if (stack === "html") return "js";
  if (path.endsWith(".tsx")) return "tsx";
  if (path.endsWith(".ts")) return "ts";
  return "jsx";
}

export function resolveProject(stack: Stack, files: Record<string, string>, importer: string, specifier: string): string | null {
  let base: string;
  if (specifier.startsWith(projectPrefix)) base = specifier.slice(projectPrefix.length);
  else if (stack === "react" && specifier.startsWith("@/")) base = posix.join("src", specifier.slice(2));
  else if (specifier.startsWith("./") || specifier.startsWith("../")) base = posix.join(posix.dirname(importer), specifier);
  else return null;
  base = posix.normalize(base);
  const root = stack === "react" ? "src/" : "scripts/";
  if (!base.startsWith(root) || base.includes("..")) return null;
  const candidates = stack === "react" ? [base, ...moduleExtensions.map((ext) => base + ext), ...moduleExtensions.map((ext) => `${base}/index${ext}`)] : [base, `${base}.js`];
  return candidates.find((candidate) => files[candidate] !== undefined) ?? null;
}

export function packageOf(specifier: string): string {
  const parts = specifier.split("/");
  return specifier.startsWith("@") && parts.length >= 2 ? `${parts[0]}/${parts[1]}` : parts[0];
}

function badSpecifier(specifier: string): boolean {
  return specifier === "" || specifier.includes("..") || specifier.includes(":") || specifier.includes("\\") || specifier.startsWith("/") || specifier.includes("\0");
}

const errorCodes = new Set([importForbidden, importDynamicForbidden]);
const outsideFile = "a file outside the project cannot be bundled";
const hiddenPath = "[path]";
const pathToken = /(^|[\s"'`(=])(?:[A-Za-z]:[\\/]|\/(?=[A-Za-z0-9_.~-])|[^\s"'`()]*\\|(?:\.\.?[\\/])*node_modules[\\/])[^\s"'`()]*/g;

export function hidePaths(text: string, roots: string[]): string {
  let out = text;
  for (const root of roots.flatMap((root) => [root, root.replaceAll("\\", "/")]).filter((root) => root.length > 1).sort((a, b) => b.length - a.length)) {
    out = out.split(root).join(hiddenPath);
  }
  return out.replace(pathToken, (_, lead: string) => lead + hiddenPath);
}

function errorsFrom(messages: readonly esbuild.Message[], files: Record<string, string>, roots: string[]): BuildError[] {
  return messages.map((message) => {
    const file = (message.location?.file ?? "").replace(/^vz(-entry)?:/, "");
    const inProject = files[file] !== undefined;
    const error: BuildError = {
      path: inProject ? file : "",
      line: inProject ? message.location?.line ?? 0 : 0,
      column: inProject ? (message.location?.column ?? 0) + 1 : 0,
      message: hidePaths(message.text, roots),
    };
    if (typeof message.detail === "string" && errorCodes.has(message.detail)) error.code = message.detail;
    return error;
  });
}

const innerResolve = { site: "inner" };
const librariesRoot = realpathSync(join(builderRoot, "node_modules")) + sep;

export function insideLibraries(path: string): boolean {
  return path.startsWith(librariesRoot) && !path.split(/[\\/]/).includes("..");
}

export async function bundle(input: BundleInput): Promise<BundleOutput> {
  const jobDir = realpathSync(mkdtempSync(join(tmpdir(), "vz-site-")));
  try {
    return await bundleIn(input, jobDir);
  } finally {
    rmSync(jobDir, { recursive: true, force: true });
  }
}

async function bundleIn(input: BundleInput, jobDir: string): Promise<BundleOutput> {
  const { stack, files, vendor } = input;
  const roots = [jobDir, builderRoot, tmpdir(), process.cwd()];
  const imports: Import[] = [];
  const seenImports = new Set<string>();
  const cssImports: string[] = [];
  const vendorStyles: string[] = [];
  const graph = new Map<string, Set<string>>();
  const usedVendor = new Map<string, string>();
  const inline = new Set(vendor.manifest.inline);
  const packageAllowed = (target: Stack, specifier: string) => target === "react" || vendor.manifest.html.includes(packageOf(specifier));
  const edge = (from: string, to: string) => {
    if (!graph.has(from)) graph.set(from, new Set());
    graph.get(from)!.add(to);
  };
  const record = (importer: string, specifier: string) => {
    const key = `${importer}\u0000${specifier}`;
    if (seenImports.has(key)) return;
    seenImports.add(key);
    imports.push({ importer, specifier });
  };
  const entryCode = new Map(input.entries.filter((e) => e.code !== undefined).map((e) => [e.name, e.code!]));
  const libraryAllowed = (specifier: string) =>
    !badSpecifier(specifier) &&
    ((vendor.manifest.modules[specifier] !== undefined && packageAllowed(stack, specifier)) ||
      vendor.manifest.styles[specifier] !== undefined ||
      (inline.has(specifier) && packageAllowed(stack, specifier)));
  const projectRelative = (specifier: string) => specifier.startsWith("./") || specifier.startsWith("../") || specifier.startsWith("@/");
  const refusal = (importer: string, specifier: string): string | null => {
    if (projectRelative(specifier)) return resolveProject(stack, files, importer, specifier) ? null : `${specifier} does not resolve to a project file`;
    if (badSpecifier(specifier)) return `import ${specifier} is not allowed`;
    return libraryAllowed(specifier) ? null : `import ${specifier} is not in the allowed library list`;
  };
  const refused = (text: string, code: string) => ({ errors: [{ text, detail: code }] });
  const plugin: esbuild.Plugin = {
    name: "site-project",
    setup(build) {
      build.onResolve({ filter: /^vz-entry:/ }, (args) => ({ path: args.path.slice(entryPrefix.length), namespace: "vz-entry" }));
      build.onResolve({ filter: /.*/ }, async (args) => {
        if (args.pluginData === innerResolve) return undefined;
        if (args.kind === "entry-point") {
          if (args.path.startsWith(projectPrefix)) return { path: args.path.slice(projectPrefix.length), namespace: "vz" };
          return undefined;
        }
        if (Object.keys(args.with ?? {}).length > 0) return refused("import attributes are not allowed", importDynamicForbidden);
        const fromProject = args.namespace === "vz" || args.namespace === "vz-entry";
        const importer = args.namespace === "vz" ? args.importer : "";
        const specifier = args.path;
        if (input.captureRoot && (specifier === captureSpecifier || (fromProject && specifier === "react-dom/client"))) {
          if (importer) record(importer, specifier);
          return { path: "client", namespace: "vz-capture" };
        }
        if (fromProject && !specifier.startsWith(projectPrefix) && importer) record(importer, specifier);
        if (!fromProject) {
          const module = vendor.manifest.modules[specifier];
          if (module && args.namespace === "file") {
            usedVendor.set(specifier, module.file);
            return { path: vendor.url(module.file), external: true };
          }
          return undefined;
        }
        const project = resolveProject(stack, files, importer, specifier);
        if (project) {
          if (importer) edge(importer, project);
          if (project.endsWith(".css")) {
            if (stack !== "react") return { errors: [{ text: `css import ${specifier} is only allowed in the react stack` }] };
            if (!cssImports.includes(project)) cssImports.push(project);
            return { path: project, namespace: "vz-empty" };
          }
          return { path: project, namespace: "vz" };
        }
        if (projectRelative(specifier) || specifier.startsWith(projectPrefix)) return refused(`${specifier} does not resolve to a project file`, importForbidden);
        if (badSpecifier(specifier)) return refused(`import ${specifier} is not allowed`, importForbidden);
        if (importer) edge(importer, specifier);
        const module = vendor.manifest.modules[specifier];
        if (module && packageAllowed(stack, specifier)) {
          usedVendor.set(specifier, module.file);
          return { path: vendor.url(module.file), external: true };
        }
        if (vendor.manifest.styles[specifier]) {
          if (!vendorStyles.includes(specifier)) vendorStyles.push(specifier);
          return { path: specifier, namespace: "vz-empty" };
        }
        if (inline.has(specifier) && packageAllowed(stack, specifier)) {
          const resolved = await build.resolve(specifier, { kind: args.kind, resolveDir: builderRoot, pluginData: innerResolve });
          if (resolved.errors.length > 0) return { errors: resolved.errors };
          if (resolved.namespace !== "file" || resolved.external) return refused(`import ${specifier} is not in the allowed library list`, importForbidden);
          return { path: resolved.path };
        }
        return refused(`import ${specifier} is not in the allowed library list`, importForbidden);
      });
      build.onLoad({ filter: /.*/, namespace: "file" }, (args) => (insideLibraries(args.path) ? undefined : refused(outsideFile, importDynamicForbidden)));
      build.onLoad({ filter: /.*/, namespace: "vz-entry" }, (args) => ({ contents: entryCode.get(args.path) ?? "", loader: "jsx", resolveDir: jobDir }));
      build.onLoad({ filter: /.*/, namespace: "vz-empty" }, () => ({ contents: "", loader: "js" }));
      build.onLoad({ filter: /.*/, namespace: "vz-capture" }, () => ({ contents: captureModule, loader: "js" }));
      build.onLoad({ filter: /.*/, namespace: "vz" }, (args) => {
        const source = files[args.path];
        if (source === undefined) return { errors: [{ text: `${args.path} is missing` }] };
        const policy = sourcePolicy(source, args.path, (specifier) => refusal(args.path, specifier));
        for (const specifier of policy.specifiers) if (!specifier.startsWith(projectPrefix)) record(args.path, specifier);
        if (policy.refusals.length > 0) {
          return {
            errors: policy.refusals.map((r) => ({ text: r.message, detail: r.code || undefined, location: { file: args.path, line: r.line, column: Math.max(0, r.column - 1) } })),
          };
        }
        const loader = loaderFor(stack, args.path);
        if (input.preview && stack === "react") {
          try {
            return { contents: tagJsx(source, args.path), loader, resolveDir: jobDir };
          } catch (error) {
            const loc = (error as { loc?: { line: number; column: number } }).loc;
            return { errors: [{ text: (error as Error).message.replace(/\s*\(\d+:\d+\)$/, ""), location: { file: args.path, line: loc?.line ?? 0, column: loc?.column ?? 0 } }] };
          }
        }
        return { contents: source, loader, resolveDir: jobDir };
      });
    },
  };
  const entryPoints = input.entries.map((entry) => ({ in: entry.code !== undefined ? entryPrefix + entry.name : projectPrefix + entry.path!, out: entry.name }));
  let result: esbuild.BuildResult<{ write: false; metafile: true }>;
  try {
    result = (await esbuild.build({
      entryPoints,
      bundle: true,
      splitting: true,
      format: "esm",
      platform: "browser",
      target: "es2020",
      write: false,
      metafile: true,
      outdir: join(builderRoot, ".out"),
      absWorkingDir: builderRoot,
      entryNames: "assets/[name]-[hash]",
      chunkNames: "assets/chunks/chunk-[hash]",
      assetNames: "assets/media/[name]-[hash]",
      jsx: "automatic",
      jsxImportSource: "react",
      minify: input.minify,
      legalComments: "none",
      define: { "process.env.NODE_ENV": '"production"' },
      logLevel: "silent",
      plugins: [plugin],
    })) as esbuild.BuildResult<{ write: false; metafile: true }>;
  } catch (error) {
    const failure = error as esbuild.BuildFailure;
    if (!failure.errors) throw error;
    return { files: [], entries: {}, imports, cssImports, vendorStyles, graph, errors: errorsFrom(failure.errors, files, roots) };
  }
  const outRoot = join(builderRoot, ".out").replaceAll("\\", "/");
  const relative = (path: string) => path.replaceAll("\\", "/").slice(outRoot.length + 1);
  const outputs = result.metafile.outputs;
  const outputKey = (rel: string) => Object.keys(outputs).find((key) => key.replaceAll("\\", "/").endsWith("/" + rel))!;
  const entries: Record<string, EntryOutput> = {};
  const entryNames = new Map(input.entries.map((entry) => [entry.code !== undefined ? `vz-entry:${entry.name}` : `vz:${entry.path}`, entry.name]));
  const files_: OutputFile[] = result.outputFiles.map((file) => ({ path: relative(file.path), contents: file.contents }));
  for (const file of files_) {
    const key = outputKey(file.path);
    const meta = outputs[key];
    if (!meta?.entryPoint) continue;
    const name = entryNames.get(meta.entryPoint);
    if (!name) continue;
    const chunks = new Set<string>();
    const vendorModules = new Set<string>();
    const visit = (k: string) => {
      for (const imported of outputs[k].imports) {
        if (imported.external) {
          const spec = [...usedVendor.entries()].find(([, f]) => vendor.url(f) === imported.path)?.[0];
          if (spec) vendorModules.add(spec);
          continue;
        }
        if (imported.kind !== "import-statement") continue;
        const rel = relative(join(builderRoot, imported.path));
        if (chunks.has(rel)) continue;
        chunks.add(rel);
        visit(imported.path);
      }
    };
    visit(key);
    entries[name] = { file: file.path, chunks: [...chunks].sort(), vendorModules: [...vendorModules].sort() };
  }
  return { files: files_, entries, imports, cssImports, vendorStyles, graph, errors: errorsFrom(result.errors, files, roots) };
}

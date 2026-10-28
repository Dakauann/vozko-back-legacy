import * as esbuild from "esbuild";
import { createHash } from "node:crypto";
import { posix } from "node:path";
import type { DefaultTreeAdapterMap } from "parse5";
import {
  contents,
  markupLanguage,
  ProjectError,
  reactEntry,
  readManifest,
  routeHtmlPath,
  styleFiles,
  tailwindEntry,
  type Project,
  type Route,
} from "../project";
import { compileCss, scanCandidates } from "../tailwind";
import { interactiveModule, lintModule, staticPackages, type BuildWarning } from "./analyze";
import { bundle, captureSpecifier, projectSpecifier, type BuildError, type BundleOutput, type EntryOutput, type Import } from "./bundle";
import {
  consentDefaultCode,
  integrationCode,
  IntegrationRefused,
  normalizeDocument,
  prerenderPlaceholder,
  renderShell,
  type BuildMode,
  type IntegrationScript,
  type ShellInput,
} from "./shell";
import { siteFiles } from "./sitefiles";
import { parseHtmlFragment, serializeFragment, tagHtml, walkElements, type Fragment } from "./tag";
import { loadVendor, vendorAssetPrefix, type Vendor } from "./vendor";

type Node = DefaultTreeAdapterMap["node"];
type Element = DefaultTreeAdapterMap["element"];

export const runtimeFile = "vz-runtime.js";
export const powFile = "pow.js";
export const bridgeFile = "vz-editor-bridge.js";
export const screenshotFile = "vz-screenshot.js";
export const prerenderPath = "__vz/prerender.html";

export interface BuildRequest {
  mode: BuildMode;
  project: Project;
  shell: ShellInput;
  runtime: Record<string, string>;
}

export interface BuiltAsset {
  path: string;
  content_type: string;
  body: Uint8Array;
}

export interface RouteOutput {
  route: string;
  html_path: string;
  interactive: boolean;
}

export interface BuildOutput {
  assets: BuiltAsset[];
  imports: Import[];
  routes: RouteOutput[];
  errors: BuildError[];
  warnings: BuildWarning[];
  prerender_path: string;
}

export class BuildRefused extends Error {}

class CssFailed extends Error {
  constructor(readonly path: string, message: string) {
    super(message);
  }
}

const encoder = new TextEncoder();

const contentTypes: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
  ".xml": "application/xml; charset=utf-8",
  ".txt": "text/plain; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".webp": "image/webp",
  ".woff2": "font/woff2",
};

function contentType(path: string): string {
  return contentTypes[posix.extname(path)] ?? "application/octet-stream";
}

function shortHash(body: Uint8Array | string): string {
  return createHash("sha256").update(body).digest("hex").slice(0, 10);
}

class Assets {
  private readonly byPath = new Map<string, BuiltAsset>();

  add(path: string, body: Uint8Array | string): string {
    const bytes = typeof body === "string" ? encoder.encode(body) : body;
    this.byPath.set(path, { path, content_type: contentType(path), body: bytes });
    return `/${path}`;
  }

  hashed(name: string, extension: string, body: Uint8Array | string): string {
    return this.add(`assets/${name}-${shortHash(body)}${extension}`, body);
  }

  list(): BuiltAsset[] {
    return [...this.byPath.values()].sort((a, b) => a.path.localeCompare(b.path));
  }
}

function failed(errors: BuildError[], warnings: BuildWarning[] = [], imports: Import[] = []): BuildOutput {
  return { assets: [], imports, routes: [], errors, warnings, prerender_path: "" };
}

function revisions(project: Project): Record<string, number> {
  const out: Record<string, number> = {};
  for (const path of Object.keys(project.files).sort()) {
    if (markupLanguage(project.stack, path)) out[path] = project.files[path].rev;
  }
  return out;
}

function runtimeAssets(request: BuildRequest, assets: Assets): { runtime: string; bridge?: string } {
  const runtime = request.runtime[runtimeFile];
  const pow = request.runtime[powFile];
  if (typeof runtime !== "string" || typeof pow !== "string") throw new BuildRefused("the runtime files are required");
  const powUrl = assets.hashed("pow", ".js", pow);
  const powName = `./${powUrl.split("/").pop()}`;
  const rewritten = runtime.replace(/(["'])\.\/pow\.js\1/g, `$1${powName}$1`);
  const result: { runtime: string; bridge?: string } = { runtime: assets.hashed("vz-runtime", ".js", rewritten) };
  if (request.mode === "preview") {
    const bridge = request.runtime[bridgeFile];
    if (typeof bridge !== "string") throw new BuildRefused("the editor bridge is required for previews");
    const screenshot = request.runtime[screenshotFile];
    if (typeof screenshot !== "string") throw new BuildRefused("the screenshot library is required for previews");
    const screenshotName = `./${assets.hashed("vz-screenshot", ".js", screenshot).split("/").pop()}`;
    result.bridge = assets.hashed("vz-editor-bridge", ".js", bridge.replace(/(["'])\.\/vz-screenshot\.js\1/g, `$1${screenshotName}$1`));
  }
  return result;
}

function integrationAssets(request: BuildRequest, assets: Assets): IntegrationScript[] {
  if (request.mode !== "publish") return [];
  const out: IntegrationScript[] = [];
  for (const kind of Object.keys(request.shell.integrations).sort()) {
    const { category, code } = integrationCode(kind, request.shell.integrations[kind], request.shell.loaders[kind]);
    out.push({ kind, category, src: assets.hashed(`vz-runtime-int-${kind.replace(/_/g, "-")}`, ".js", code) });
  }
  return out;
}

function vendorFiles(vendor: Vendor, specs: Iterable<string>): string[] {
  const files = new Set<string>();
  for (const spec of specs) {
    const module = vendor.manifest.modules[spec];
    if (!module) continue;
    files.add(module.file);
    for (const file of module.preload) files.add(file);
  }
  return [...files].sort();
}

async function stylesheet(request: BuildRequest, files: Record<string, string>, entries: string[], candidates: string[], assets: Assets): Promise<string | null> {
  const styles = styleFiles(request.project.stack, files);
  let css = "";
  for (const entry of entries) {
    try {
      css += await compileCss(entry, styles, candidates);
    } catch (error) {
      throw new CssFailed(entry, error instanceof Error ? error.message : String(error));
    }
  }
  if (css.trim() === "") return null;
  if (request.mode === "publish") css = (await esbuild.transform(css, { loader: "css", minify: true, legalComments: "none" })).code;
  return assets.hashed("index", ".css", css);
}

function extensionOf(path: string): string {
  return posix.extname(path).slice(1) || "html";
}

function consentSlot(request: BuildRequest, integrations: IntegrationScript[], body: string): string {
  if (integrations.length === 0 || body.includes("<vz-consent")) return "";
  const text = request.shell.consent_text ? ` text="${request.shell.consent_text.replace(/&/g, "&amp;").replace(/"/g, "&quot;")}"` : "";
  return `<vz-consent${text}></vz-consent>`;
}

interface Common {
  request: BuildRequest;
  files: Record<string, string>;
  lang: string;
  siteName: string;
  routes: Route[];
  assets: Assets;
  vendor: Vendor;
  consentDefault?: string;
  runtime: { runtime: string; bridge?: string };
  integrations: IntegrationScript[];
}

function addSiteFiles(common: Common) {
  if (common.request.mode !== "publish") return;
  for (const file of siteFiles(common.siteName, common.routes, common.request.shell)) common.assets.add(file.path, file.body);
}

function addVendorAssets(common: Common, specs: Iterable<string>) {
  for (const file of vendorFiles(common.vendor, specs)) common.assets.add(vendorAssetPrefix + file, common.vendor.read(file));
}

function addBundleFiles(common: Common, out: BundleOutput) {
  for (const file of out.files) common.assets.add(file.path, file.contents);
}

function interactiveRoute(graph: Map<string, Set<string>>, files: Record<string, string>, start: string, otherPages: Set<string>): boolean {
  const seen = new Set<string>();
  const queue = [start];
  while (queue.length > 0) {
    const current = queue.shift()!;
    if (seen.has(current) || otherPages.has(current)) continue;
    seen.add(current);
    if (files[current] === undefined) {
      if (!staticPackages.has(current)) return true;
      continue;
    }
    if (!current.endsWith(".css") && interactiveModule(files[current], current)) return true;
    for (const next of graph.get(current) ?? []) queue.push(next);
  }
  return false;
}

function prerenderEntryCode(main: string, routes: Route[]): string {
  return [
    `import { capturedElement } from ${JSON.stringify(captureSpecifier)};`,
    `import ${JSON.stringify(projectSpecifier(main))};`,
    `import { prerender } from "react-dom/static";`,
    `const vzRoutes = ${JSON.stringify(routes.map((route) => route.path))};`,
    `window.__vzPrerender = async (route) => {`,
    `  if (!vzRoutes.includes(route)) throw new Error("unknown route " + route);`,
    `  const app = capturedElement();`,
    `  if (!app) throw new Error("the entry did not render into #root");`,
    `  window.history.replaceState(null, "", route);`,
    `  const { prelude } = await prerender(app);`,
    `  return await new Response(prelude).text();`,
    `};`,
  ].join("\n");
}

function preloadsOf(vendor: Vendor, entry: EntryOutput): string[] {
  return [...entry.chunks.map((chunk) => `/${chunk}`), ...vendorFiles(vendor, entry.vendorModules).map((file) => vendor.url(file))];
}

async function buildReact(common: Common): Promise<BuildOutput> {
  const { request, files, assets, vendor } = common;
  const main = reactEntry(files);
  if (!main) return failed([{ path: "src/main.jsx", line: 0, column: 0, message: "the project needs exactly one src/main.jsx or src/main.tsx" }]);
  const publish = request.mode === "publish";
  const out = await bundle({ stack: "react", files, entries: [{ name: "app", path: main }], preview: !publish, minify: publish, vendor });
  const modules = Object.keys(files).filter((path) => markupLanguage("react", path) === "jsx").sort();
  const warnings = modules.flatMap((path) => lintModule(files[path], path));
  if (out.errors.length > 0) return failed(out.errors, warnings, out.imports);
  const prerenderOut = publish
    ? await bundle({ stack: "react", files, entries: [{ name: "prerender", code: prerenderEntryCode(main, common.routes) }], preview: false, minify: true, vendor, captureRoot: true })
    : null;
  if (prerenderOut && prerenderOut.errors.length > 0) return failed(prerenderOut.errors, warnings, out.imports);
  const candidates = scanCandidates(modules.map((path) => ({ content: files[path], extension: extensionOf(path) })));
  const cssEntries = out.cssImports.length > 0 ? out.cssImports : [tailwindEntry("react", files)].filter((path): path is string => path !== null);
  const css = await stylesheet(request, files, cssEntries, candidates, assets);
  addBundleFiles(common, out);
  const app = out.entries.app;
  const vendorSpecs = new Set(app.vendorModules);
  if (prerenderOut) {
    addBundleFiles(common, prerenderOut);
    for (const spec of prerenderOut.entries.prerender.vendorModules) vendorSpecs.add(spec);
  }
  addVendorAssets(common, vendorSpecs);
  const vendorStyles = out.vendorStyles.map((spec) => assets.add(vendorAssetPrefix + vendor.manifest.styles[spec], vendor.read(vendor.manifest.styles[spec])));
  const styles = [...vendorStyles, ...(css ? [css] : [])];
  const pages = new Set(common.routes.map((route) => route.page));
  const routes: RouteOutput[] = [];
  for (const route of common.routes) {
    const root = publish ? `<div id="root">${prerenderPlaceholder}</div>` : `<div id="root"></div>`;
    const html = renderShell({
      mode: request.mode,
      lang: common.lang,
      siteName: common.siteName,
      route: route.path,
      shell: request.shell,
      revs: revisions(request.project),
      styles,
      preloads: preloadsOf(vendor, app),
      consentDefault: common.consentDefault,
      runtime: common.runtime.runtime,
      bridge: common.runtime.bridge,
      integrations: common.integrations,
      body: root + consentSlot(request, common.integrations, ""),
      appScripts: [`/${app.file}`],
    });
    const htmlPath = routeHtmlPath(route.path);
    assets.add(htmlPath, html);
    const others = new Set([...pages].filter((page) => page !== route.page));
    routes.push({ route: route.path, html_path: htmlPath, interactive: publish ? interactiveRoute(out.graph, files, main, others) : true });
  }
  let prerender = "";
  if (prerenderOut) {
    const entry = prerenderOut.entries.prerender;
    const page = `<!doctype html><html><head><meta charset="utf-8">${preloadsOf(vendor, entry).map((href) => `<link rel="modulepreload" href="${href}">`).join("")}<script type="module" src="/${entry.file}"></script></head><body><div id="root"></div></body></html>`;
    assets.add(prerenderPath, normalizeDocument(page));
    prerender = prerenderPath;
  }
  addSiteFiles(common);
  return { assets: assets.list(), imports: out.imports, routes, errors: [], warnings, prerender_path: prerender };
}

const maxIncludeDepth = 8;

function includeTarget(src: string | undefined): string | null {
  if (!src) return null;
  const path = src.replace(/^\.?\//, "");
  return /^partials\/[a-z0-9][a-z0-9_-]*\.html$/.test(path) ? path : null;
}

function expandIncludes(common: Common, fragment: Fragment, page: string, chain: string[], errors: BuildError[]) {
  const preview = common.request.mode === "preview";
  walkElements(fragment as unknown as Node, (el: Element, parent: Node) => {
    if (el.tagName !== "vz-include") return;
    const src = el.attrs.find((attr) => attr.name === "src")?.value;
    const target = includeTarget(src);
    const siblings = (parent as { childNodes: Node[] }).childNodes;
    const at = siblings.indexOf(el as unknown as Node);
    if (!target || common.files[target] === undefined) {
      errors.push({ path: page, line: el.sourceCodeLocation?.startLine ?? 0, column: el.sourceCodeLocation?.startCol ?? 0, message: `vz-include ${src ?? ""} does not point to a partial` });
      siblings.splice(at, 1);
      return;
    }
    if (chain.includes(target) || chain.length >= maxIncludeDepth) {
      errors.push({ path: page, line: 0, column: 0, message: `vz-include ${target} includes itself` });
      siblings.splice(at, 1);
      return;
    }
    const partial = parseHtmlFragment(common.files[target], preview);
    if (preview) tagHtml(partial, target);
    expandIncludes(common, partial, target, [...chain, target], errors);
    const children = partial.childNodes as Node[];
    for (const child of children) (child as { parentNode: unknown }).parentNode = parent;
    siblings.splice(at, 1, ...children);
  });
}

function scriptTarget(files: Record<string, string>, src: string | undefined): string | null {
  if (!src) return null;
  const path = src.replace(/^\.?\//, "");
  return /^scripts\/[a-z0-9][a-z0-9_-]*\.js$/.test(path) && files[path] !== undefined ? path : null;
}

async function buildHtml(common: Common): Promise<BuildOutput> {
  const { request, files, assets, vendor } = common;
  const publish = request.mode === "publish";
  const scripts = Object.keys(files).filter((path) => /^scripts\/[a-z0-9][a-z0-9_-]*\.js$/.test(path)).sort();
  const scriptEntries = scripts.map((path) => ({ name: `script-${posix.basename(path, ".js")}`, path }));
  const out: BundleOutput = scriptEntries.length > 0
    ? await bundle({ stack: "html", files, entries: scriptEntries, preview: !publish, minify: publish, vendor })
    : { files: [], entries: {}, imports: [], cssImports: [], vendorStyles: [], graph: new Map(), errors: [] };
  const errors = [...out.errors];
  const markup = Object.keys(files).filter((path) => markupLanguage("html", path) === "html").sort();
  const candidates = scanCandidates([
    ...markup.map((path) => ({ content: files[path], extension: "html" })),
    ...scripts.map((path) => ({ content: files[path], extension: "js" })),
  ]);
  const bodies = new Map<string, { body: string; interactive: boolean; vendorModules: Set<string>; preloads: string[] }>();
  for (const route of common.routes) {
    const fragment = parseHtmlFragment(files[route.page], true);
    if (!publish) tagHtml(fragment, route.page);
    expandIncludes(common, fragment, route.page, [route.page], errors);
    let interactive = false;
    const vendorModules = new Set<string>();
    const preloads: string[] = [];
    walkElements(fragment as unknown as Node, (el: Element) => {
      if (el.tagName !== "script") return;
      const src = el.attrs.find((attr) => attr.name === "src");
      const target = scriptTarget(files, src?.value);
      if (!src || !target) return;
      const entry = out.entries[`script-${posix.basename(target, ".js")}`];
      if (!entry) return;
      interactive = true;
      src.value = `/${entry.file}`;
      if (!el.attrs.some((attr) => attr.name === "type")) el.attrs.push({ name: "type", value: "module" });
      for (const spec of entry.vendorModules) vendorModules.add(spec);
      preloads.push(...entry.chunks.map((chunk) => `/${chunk}`));
    });
    bodies.set(route.path, { body: serializeFragment(fragment), interactive, vendorModules, preloads });
  }
  if (errors.length > 0) return failed(errors, [], out.imports);
  const entry = tailwindEntry("html", files);
  const css = entry ? await stylesheet(request, files, [entry], candidates, assets) : null;
  addBundleFiles(common, out);
  addVendorAssets(common, Object.values(out.entries).flatMap((e) => e.vendorModules));
  const routes: RouteOutput[] = [];
  for (const route of common.routes) {
    const page = bodies.get(route.path)!;
    const preloads = [...new Set([...page.preloads, ...vendorFiles(vendor, page.vendorModules).map((file) => vendor.url(file))])];
    const html = renderShell({
      mode: request.mode,
      lang: common.lang,
      siteName: common.siteName,
      route: route.path,
      shell: request.shell,
      revs: revisions(request.project),
      styles: css ? [css] : [],
      preloads,
      consentDefault: common.consentDefault,
      runtime: common.runtime.runtime,
      bridge: common.runtime.bridge,
      integrations: common.integrations,
      body: page.body + consentSlot(request, common.integrations, page.body),
      appScripts: [],
    });
    const htmlPath = routeHtmlPath(route.path);
    assets.add(htmlPath, html);
    routes.push({ route: route.path, html_path: htmlPath, interactive: page.interactive });
  }
  addSiteFiles(common);
  return { assets: assets.list(), imports: out.imports, routes, errors: [], warnings: [], prerender_path: "" };
}

export async function build(request: BuildRequest): Promise<BuildOutput> {
  const files = contents(request.project);
  let manifest;
  try {
    manifest = readManifest(files);
  } catch (error) {
    if (error instanceof ProjectError) return failed([{ path: "site.json", line: 0, column: 0, message: error.message }]);
    throw error;
  }
  const assets = new Assets();
  let integrations: IntegrationScript[];
  try {
    integrations = integrationAssets(request, assets);
  } catch (error) {
    if (error instanceof IntegrationRefused) throw new BuildRefused(error.message);
    throw error;
  }
  const common: Common = {
    request,
    files,
    lang: manifest.lang,
    siteName: manifest.name,
    routes: manifest.routes,
    assets,
    vendor: loadVendor(),
    consentDefault: integrations.length > 0 ? assets.hashed("vz-runtime-consent", ".js", consentDefaultCode) : undefined,
    runtime: runtimeAssets(request, assets),
    integrations,
  };
  try {
    return request.project.stack === "react" ? await buildReact(common) : await buildHtml(common);
  } catch (error) {
    if (error instanceof CssFailed) return failed([{ path: error.path, line: 0, column: 0, message: error.message }]);
    throw error;
  }
}

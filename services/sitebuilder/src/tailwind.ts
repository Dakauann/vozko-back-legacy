import { compile, __unstable__loadDesignSystem } from "tailwindcss";
import { extendTailwindMerge, twMerge } from "tailwind-merge";
import { Scanner } from "@tailwindcss/oxide";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { posix } from "node:path";
import { createHash } from "node:crypto";

const projectRoot = "/project";
const require = createRequire(import.meta.url);
const tailwindDir = posix.dirname(require.resolve("tailwindcss/package.json").replaceAll("\\", "/"));

const tailwindSheets: Record<string, string> = {
  tailwindcss: "index.css",
  "tailwindcss/index.css": "index.css",
  "tailwindcss/theme": "theme.css",
  "tailwindcss/theme.css": "theme.css",
  "tailwindcss/preflight": "preflight.css",
  "tailwindcss/preflight.css": "preflight.css",
  "tailwindcss/utilities": "utilities.css",
  "tailwindcss/utilities.css": "utilities.css",
};

const sheetCache = new Map<string, string>();

function tailwindSheet(id: string): string | null {
  const file = tailwindSheets[id];
  if (!file) return null;
  let content = sheetCache.get(file);
  if (content === undefined) {
    content = readFileSync(`${tailwindDir}/${file}`, "utf8");
    sheetCache.set(file, content);
  }
  return content;
}

export class StylesheetRefused extends Error {}

export type StyleFiles = Record<string, string>;

function loaders(styles: StyleFiles) {
  return {
    async loadStylesheet(id: string, base: string) {
      const builtIn = tailwindSheet(id);
      if (builtIn !== null) return { path: `${tailwindDir}/${tailwindSheets[id]}`, base: tailwindDir, content: builtIn };
      if (!id.startsWith("./") && !id.startsWith("../")) throw new StylesheetRefused(`stylesheet ${id} is not allowed`);
      const resolved = posix.normalize(posix.join(base, id));
      if (!resolved.startsWith(projectRoot + "/")) throw new StylesheetRefused(`stylesheet ${id} is outside the project`);
      const path = resolved.slice(projectRoot.length + 1);
      const content = styles[path];
      if (content === undefined) throw new StylesheetRefused(`stylesheet ${path} does not exist`);
      return { path: resolved, base: posix.dirname(resolved), content };
    },
    async loadModule(id: string): Promise<never> {
      throw new StylesheetRefused(`module ${id} cannot be loaded`);
    },
  };
}

function hash(parts: readonly string[]): string {
  const h = createHash("sha256");
  for (const part of parts) h.update(part).update("\u0000");
  return h.digest("hex");
}

class Lru<V> {
  private readonly items = new Map<string, V>();
  constructor(private readonly limit: number) {}
  get(key: string): V | undefined {
    const value = this.items.get(key);
    if (value !== undefined) {
      this.items.delete(key);
      this.items.set(key, value);
    }
    return value;
  }
  set(key: string, value: V) {
    this.items.delete(key);
    this.items.set(key, value);
    while (this.items.size > this.limit) this.items.delete(this.items.keys().next().value!);
  }
}

type Compiler = Awaited<ReturnType<typeof compile>>;
type DesignSystem = Awaited<ReturnType<typeof __unstable__loadDesignSystem>>;

const compilers = new Lru<Promise<Compiler>>(32);
const designSystems = new Lru<Promise<DesignSystem>>(32);
const outputs = new Lru<string>(64);

function entryKey(entry: string, styles: StyleFiles): string {
  return hash([entry, ...Object.keys(styles).sort().flatMap((path) => [path, styles[path]])]);
}

function baseOf(entry: string): string {
  return posix.join(projectRoot, posix.dirname(entry));
}

export async function compileCss(entry: string, styles: StyleFiles, candidates: readonly string[]): Promise<string> {
  const key = entryKey(entry, styles);
  let compiler = compilers.get(key);
  if (!compiler) {
    compiler = compile(styles[entry], { base: baseOf(entry), ...loaders(styles) });
    compilers.set(key, compiler);
    compiler.catch(() => compilers.set(key, undefined as never));
  }
  const ready = await compiler;
  const sorted = [...new Set(candidates)].sort();
  const outputKey = hash([key, ...sorted]);
  const cached = outputs.get(outputKey);
  if (cached !== undefined) return cached;
  const css = ready.build(sorted);
  outputs.set(outputKey, css);
  return css;
}

async function designSystem(entry: string, styles: StyleFiles): Promise<DesignSystem> {
  const key = entryKey(entry, styles);
  let system = designSystems.get(key);
  if (!system) {
    system = __unstable__loadDesignSystem(styles[entry], { base: baseOf(entry), ...loaders(styles) });
    designSystems.set(key, system);
    system.catch(() => designSystems.set(key, undefined as never));
  }
  return system;
}

function stripCss(content: string): string {
  return content.replace(/\/\*[\s\S]*?\*\//g, "").replace(/"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'/g, '""');
}

export function declaredClasses(styles: StyleFiles): Set<string> {
  const out = new Set<string>();
  for (const content of Object.values(styles)) {
    for (const match of stripCss(content).matchAll(/\.(-?[A-Za-z_][\w-]*)/g)) out.add(match[1]);
  }
  return out;
}

export async function classValidator(entry: string | null, styles: StyleFiles): Promise<(cls: string) => boolean> {
  const declared = declaredClasses(styles);
  if (entry === null) return (cls) => declared.has(cls);
  const system = await designSystem(entry, styles);
  return (cls) => {
    const bare = cls.slice(cls.lastIndexOf(":") + 1);
    if (declared.has(bare) && bare === cls) return true;
    const [css] = system.candidatesToCss([cls]);
    return typeof css === "string" && css.length > 0;
  };
}

const mergeNamespaces = [
  "font-weight",
  "inset-shadow",
  "drop-shadow",
  "text-shadow",
  "animate",
  "aspect",
  "blur",
  "breakpoint",
  "color",
  "container",
  "ease",
  "font",
  "leading",
  "perspective",
  "radius",
  "shadow",
  "spacing",
  "text",
  "tracking",
];

export function themeMergeConfig(variables: Iterable<string>): Record<string, string[]> {
  const out: Record<string, string[]> = {};
  for (const variable of variables) {
    if (!variable.startsWith("--")) continue;
    const name = variable.slice(2);
    const namespace = mergeNamespaces.find((candidate) => name.startsWith(candidate + "-"));
    if (!namespace) continue;
    const value = name.slice(namespace.length + 1);
    if (value === "" || value.includes("--")) continue;
    (out[namespace] ??= []).push(value);
  }
  for (const values of Object.values(out)) values.sort();
  return out;
}

export type ClassMerger = (classes: string) => string;

const mergers = new Lru<Promise<ClassMerger>>(32);

export async function classMerger(entry: string | null, styles: StyleFiles): Promise<ClassMerger> {
  if (entry === null) return twMerge;
  const key = entryKey(entry, styles);
  let merger = mergers.get(key);
  if (!merger) {
    merger = designSystem(entry, styles).then((system) => {
      const theme = themeMergeConfig([...system.theme.entries()].map(([name]) => name));
      return extendTailwindMerge({ extend: { theme } }) as ClassMerger;
    });
    mergers.set(key, merger);
    merger.catch(() => mergers.set(key, undefined as never));
  }
  return merger;
}

export interface ScanFile {
  content: string;
  extension: string;
}

export function scanCandidates(files: readonly ScanFile[]): string[] {
  if (files.length === 0) return [];
  const scanner = new Scanner({});
  return scanner.scanFiles(files.map((file) => ({ content: file.content, extension: file.extension })));
}

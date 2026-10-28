export type Stack = "react" | "html";

export interface ProjectFile {
  content: string;
  rev: number;
}

export interface Project {
  stack: Stack;
  files: Record<string, ProjectFile>;
}

export interface Route {
  path: string;
  page: string;
}

export interface SiteManifest {
  name: string;
  lang: string;
  routes: Route[];
}

export const manifestPath = "site.json";
export const knobsPath = "knobs.json";
export const htmlThemePath = "theme.css";
export const reactEntries = ["src/main.jsx", "src/main.tsx"];
export const defaultLang = "pt-BR";

export class ProjectError extends Error {}

export type MarkupLanguage = "html" | "jsx";

export function markupLanguage(stack: Stack, path: string): MarkupLanguage | null {
  if (stack === "html") return /^(pages|partials)\/[a-z0-9][a-z0-9_-]*\.html$/.test(path) ? "html" : null;
  return /^src\/.+\.(jsx|tsx|js|ts)$/.test(path) ? "jsx" : null;
}

export function isStyle(stack: Stack, path: string): boolean {
  if (stack === "html") return path === htmlThemePath;
  return /^src\/.+\.css$/.test(path);
}

export function styleFiles(stack: Stack, files: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [path, content] of Object.entries(files)) if (isStyle(stack, path)) out[path] = content;
  return out;
}

const tailwindImport = /@import\s+["']tailwindcss["']/;
const themeBlock = /@theme\b/;

export function themeFile(stack: Stack, files: Record<string, string>): string | null {
  if (stack === "html") return files[htmlThemePath] !== undefined ? htmlThemePath : null;
  const styles = Object.keys(styleFiles(stack, files)).sort();
  const preferred = ["src/index.css", ...styles];
  return preferred.find((path) => files[path] !== undefined && themeBlock.test(files[path]))
    ?? preferred.find((path) => files[path] !== undefined && tailwindImport.test(files[path]))
    ?? null;
}

export function tailwindEntry(stack: Stack, files: Record<string, string>): string | null {
  if (stack === "html") return files[htmlThemePath] !== undefined ? htmlThemePath : null;
  const styles = Object.keys(styleFiles(stack, files)).sort();
  return ["src/index.css", ...styles].find((path) => files[path] !== undefined && tailwindImport.test(files[path])) ?? null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function readManifest(files: Record<string, string>): SiteManifest {
  const raw = files[manifestPath];
  if (raw === undefined) throw new ProjectError("site.json is missing");
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new ProjectError("site.json is not valid JSON");
  }
  if (!isRecord(parsed) || typeof parsed.name !== "string" || !Array.isArray(parsed.routes)) throw new ProjectError("site.json needs a name and routes");
  const routes: Route[] = [];
  for (const route of parsed.routes) {
    if (!isRecord(route) || typeof route.path !== "string" || typeof route.page !== "string") throw new ProjectError("every route needs a path and a page");
    if (!/^\/([a-z0-9]+(-[a-z0-9]+)*(\/[a-z0-9]+(-[a-z0-9]+)*)*)?$/.test(route.path)) throw new ProjectError(`route ${route.path} is not a lowercase path`);
    if (files[route.page] === undefined) throw new ProjectError(`route ${route.path} points to the missing page ${route.page}`);
    routes.push({ path: route.path, page: route.page });
  }
  if (!routes.some((route) => route.path === "/")) throw new ProjectError("the home route / is required");
  const lang = typeof parsed.lang === "string" && /^[a-z]{2}(-[A-Z]{2})?$/.test(parsed.lang) ? parsed.lang : defaultLang;
  return { name: parsed.name, lang, routes };
}

export function reactEntry(files: Record<string, string>): string | null {
  const present = reactEntries.filter((path) => files[path] !== undefined);
  return present.length === 1 ? present[0] : null;
}

export function contents(project: Project): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [path, file] of Object.entries(project.files)) out[path] = file.content;
  return out;
}

export function routeHtmlPath(route: string): string {
  return route === "/" ? "index.html" : `${route.slice(1)}/index.html`;
}

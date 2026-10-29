import type { BuildRequest } from "./build";
import type { PageMeta, ShellInput } from "./build/shell";
import type { Run } from "./execute/inline";
import { operationKinds, type ExecuteRequest, type ExtractRequest, type FileBase, type Operation, type Placement } from "./execute/types";
import type { Project, ProjectFile } from "./project";

export class RequestInvalid extends Error {}

type Json = Record<string, unknown>;

function fail(where: string, reason: string): never {
  throw new RequestInvalid(`${where} ${reason}`);
}

function isObject(value: unknown): value is Json {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function object(value: unknown, where: string, allowed: readonly string[]): Json {
  if (!isObject(value)) fail(where, "must be an object");
  for (const key of Object.keys(value)) if (!allowed.includes(key)) fail(where, `has the unknown field ${key}`);
  return value;
}

function record<T>(value: unknown, where: string, item: (v: unknown, at: string) => T): Record<string, T> {
  if (!isObject(value)) fail(where, "must be an object");
  const out: Record<string, T> = {};
  for (const [key, entry] of Object.entries(value)) out[key] = item(entry, `${where}.${key}`);
  return out;
}

function string(value: unknown, where: string): string {
  if (typeof value !== "string") fail(where, "must be a string");
  return value;
}

function optionalString(value: unknown, where: string): string | undefined {
  return value === undefined ? undefined : string(value, where);
}

function integer(value: unknown, where: string, min: number): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < min) fail(where, `must be an integer of at least ${min}`);
  return value;
}

function strings(value: unknown, where: string): string[] {
  if (!Array.isArray(value)) fail(where, "must be a list");
  return value.map((entry, i) => string(entry, `${where}[${i}]`));
}

function project(value: unknown): Project {
  const raw = object(value, "project", ["stack", "files"]);
  if (raw.stack !== "react" && raw.stack !== "html") fail("project.stack", "must be react or html");
  const files = record<ProjectFile>(raw.files, "project.files", (entry, at) => {
    const file = object(entry, at, ["content", "rev"]);
    return { content: string(file.content, `${at}.content`), rev: integer(file.rev, `${at}.rev`, 1) };
  });
  return { stack: raw.stack, files };
}

const operationFields = [
  "op", "path", "to", "content", "old", "new", "replace_all", "node", "rev", "item", "text", "name", "value", "add", "remove",
  "breakpoint", "media_id", "alt", "before", "after", "into", "at", "html", "library_key", "ref", "source", "source_rev",
] as const;

function placement(value: unknown, where: string): Placement {
  const raw = object(value, where, ["before", "after", "into", "page"]);
  const out: Placement = {};
  for (const key of ["before", "after", "into", "page"] as const) if (raw[key] !== undefined) out[key] = string(raw[key], `${where}.${key}`);
  return out;
}

function operation(value: unknown, where: string): Operation {
  const raw = object(value, where, operationFields);
  if (typeof raw.op !== "string" || !operationKinds.includes(raw.op as Operation["op"])) fail(`${where}.op`, "is not a known operation");
  const out: Operation = { op: raw.op as Operation["op"] };
  for (const key of ["path", "to", "content", "old", "new", "node", "text", "name", "breakpoint", "media_id", "alt", "before", "after", "into", "html", "library_key", "ref", "source"] as const) {
    if (raw[key] !== undefined) out[key] = string(raw[key], `${where}.${key}`);
  }
  if (raw.replace_all !== undefined) {
    if (typeof raw.replace_all !== "boolean") fail(`${where}.replace_all`, "must be a boolean");
    out.replace_all = raw.replace_all;
  }
  if (raw.rev !== undefined) out.rev = integer(raw.rev, `${where}.rev`, 1);
  if (raw.source_rev !== undefined) out.source_rev = integer(raw.source_rev, `${where}.source_rev`, 1);
  if (raw.item !== undefined) out.item = integer(raw.item, `${where}.item`, 0);
  if (raw.value !== undefined) {
    if (typeof raw.value !== "string" && (typeof raw.value !== "number" || !Number.isFinite(raw.value))) fail(`${where}.value`, "must be a string or a number");
    out.value = raw.value;
  }
  if (raw.add !== undefined) out.add = strings(raw.add, `${where}.add`);
  if (raw.remove !== undefined) out.remove = strings(raw.remove, `${where}.remove`);
  if (raw.at !== undefined) out.at = placement(raw.at, `${where}.at`);
  return out;
}

function run(value: unknown, where: string): Run {
  const raw = object(value, where, ["text", "bold", "italic", "underline", "link"]);
  const out: Run = { text: string(raw.text, `${where}.text`) };
  for (const key of ["bold", "italic", "underline"] as const) {
    if (raw[key] === undefined) continue;
    if (typeof raw[key] !== "boolean") fail(`${where}.${key}`, "must be a boolean");
    out[key] = raw[key] as boolean;
  }
  if (raw.link !== undefined) out.link = { href: string(object(raw.link, `${where}.link`, ["href"]).href, `${where}.link.href`) };
  return out;
}

export function decodeExecute(value: unknown): ExecuteRequest {
  const raw = object(value, "request", ["project", "operations", "texts", "media", "sections", "jsx_sections", "bases"]);
  if (!Array.isArray(raw.operations) || raw.operations.length === 0) fail("operations", "must be a non empty list");
  return {
    project: project(raw.project),
    operations: raw.operations.map((entry, i) => operation(entry, `operations[${i}]`)),
    texts: raw.texts === undefined ? {} : record(raw.texts, "texts", (entry, at) => {
      if (!Array.isArray(entry)) fail(at, "must be a list of runs");
      return entry.map((r, i) => run(r, `${at}[${i}]`));
    }),
    media: raw.media === undefined ? {} : record(raw.media, "media", (entry, at) => {
      const url = string(entry, at);
      if (!/^https:\/\/[^\s"'<>]+$/.test(url)) fail(at, "must be an https url");
      return url;
    }),
    sections: raw.sections === undefined ? {} : record(raw.sections, "sections", string),
    jsx_sections: raw.jsx_sections === undefined ? {} : record(raw.jsx_sections, "jsx_sections", string),
    bases: bases(raw.bases),
  };
}

function bases(value: unknown): Record<string, FileBase[]> {
  return value === undefined ? {} : record(value, "bases", (entry, at) => {
    if (!Array.isArray(entry)) fail(at, "must be a list");
    return entry.map((base, i): FileBase => {
      const b = object(base, `${at}[${i}]`, ["rev", "content"]);
      return { rev: integer(b.rev, `${at}[${i}].rev`, 1), content: string(b.content, `${at}[${i}].content`) };
    });
  });
}

export function decodeExtract(value: unknown): ExtractRequest {
  const raw = object(value, "request", ["project", "node", "rev", "bases"]);
  const out: ExtractRequest = { project: project(raw.project), node: string(raw.node, "node"), bases: bases(raw.bases) };
  if (raw.rev !== undefined) out.rev = integer(raw.rev, "rev", 1);
  return out;
}

function page(value: unknown, where: string): PageMeta {
  const raw = object(value, where, ["title", "description", "canonical", "og_image", "noindex"]);
  const out: PageMeta = {};
  for (const key of ["title", "description", "canonical", "og_image"] as const) if (raw[key] !== undefined) out[key] = string(raw[key], `${where}.${key}`);
  if (raw.noindex !== undefined) {
    if (typeof raw.noindex !== "boolean") fail(`${where}.noindex`, "must be a boolean");
    out.noindex = raw.noindex;
  }
  return out;
}

function parsedURL(text: string): URL | null {
  try {
    return new URL(text);
  } catch {
    return null;
  }
}

function httpsURL(text: string): boolean {
  const url = parsedURL(text);
  return url !== null && url.protocol === "https:" && url.username === "" && url.password === "" && !/[ "'<>]/.test(text);
}

function httpsOrigin(text: string): boolean {
  const url = parsedURL(text);
  return httpsURL(text) && url !== null && url.origin === text;
}

function origin(value: unknown): string | undefined {
  if (value === undefined) return undefined;
  const text = string(value, "shell.primary_origin");
  if (!httpsOrigin(text)) fail("shell.primary_origin", "must be an https origin");
  return text;
}

function loader(value: unknown, where: string): string {
  const text = string(value, where);
  const url = parsedURL(text);
  if (!httpsURL(text) || url === null || url.search !== "" || url.hash !== "" || /[?#]/.test(text)) fail(where, "must be an exact https url");
  return text;
}

function favicon(value: unknown): string | undefined {
  if (value === undefined) return undefined;
  const text = string(value, "shell.favicon");
  if (!httpsURL(text)) fail("shell.favicon", "must be an https url");
  return text;
}

function webchat(value: unknown): string | undefined {
  if (value === undefined) return undefined;
  const text = string(value, "shell.webchat");
  if (!/^[A-Za-z0-9_-]{1,64}$/.test(text)) fail("shell.webchat", "must be a widget reference");
  return text;
}

function structuredData(value: unknown): unknown[] | undefined {
  if (value === undefined) return undefined;
  if (!Array.isArray(value)) fail("shell.structured_data", "must be a list");
  value.forEach((entry, i) => {
    if (!isObject(entry)) fail(`shell.structured_data[${i}]`, "must be an object");
  });
  return value;
}

function shell(value: unknown): ShellInput {
  const raw = object(value, "shell", ["api", "forms", "variant", "pages", "integrations", "loaders", "consent_text", "primary_origin", "favicon", "structured_data", "webchat"]);
  const api = string(raw.api, "shell.api");
  if (!/^https:\/\/[a-z0-9.-]+(:\d+)?$/.test(api) && !/^http:\/\/(localhost|127\.0\.0\.1)(:\d+)?$/.test(api)) fail("shell.api", "must be an https origin");
  return {
    api,
    forms: raw.forms === undefined ? {} : record(raw.forms, "shell.forms", string),
    variant: optionalString(raw.variant, "shell.variant"),
    pages: raw.pages === undefined ? {} : record(raw.pages, "shell.pages", page),
    integrations: raw.integrations === undefined ? {} : record(raw.integrations, "shell.integrations", string),
    loaders: raw.loaders === undefined ? {} : record(raw.loaders, "shell.loaders", loader),
    consent_text: optionalString(raw.consent_text, "shell.consent_text"),
    primary_origin: origin(raw.primary_origin),
    favicon: favicon(raw.favicon),
    structured_data: structuredData(raw.structured_data),
    webchat: webchat(raw.webchat),
  };
}

export function decodeBuild(value: unknown): BuildRequest {
  const raw = object(value, "request", ["mode", "project", "shell", "runtime"]);
  if (raw.mode !== "preview" && raw.mode !== "publish") fail("mode", "must be preview or publish");
  return { mode: raw.mode, project: project(raw.project), shell: shell(raw.shell), runtime: record(raw.runtime, "runtime", string) };
}

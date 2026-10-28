import { contains, lineBlock, type MarkupElement } from "../markup/element";
import { parseHtml, type HtmlDocument } from "../markup/html";
import { parseJsx, type JsxDocument } from "../markup/jsx";
import { knobsPath, markupLanguage, styleFiles, tailwindEntry, themeFile, type Project, type Stack } from "../project";
import { classMerger, classValidator, type ClassMerger } from "../tailwind";
import type { MarkupAdapter, Workspace } from "./adapter";
import { diffSplices } from "./diff";
import { locateEdit } from "./edit";
import { htmlAdapter } from "./html-adapter";
import { jsxAdapter } from "./jsx-adapter";
import { mapOffset, Patcher, type Splice } from "./patcher";
import { afterInsertion, beforeInsertion, intoInsertion, placeText } from "./placement";
import { setCssVariable, setThemeToken } from "./theme";
import {
  OpFailure,
  type ExecuteRequest,
  type ExecuteResult,
  type ExtractRequest,
  type ExtractResult,
  type Failure,
  type FileChange,
  type OpChange,
  type Operation,
  type Skip,
  type StepPatch,
  type StepSplice,
} from "./types";

interface SectionSource {
  markup: string;
  language: "html" | "jsx";
}

interface Version {
  rev: number;
  content: string;
}

interface Transition {
  splices: Splice[];
  moved?: { from: number; to: number };
}

interface History {
  versions: Version[];
  transitions: Transition[];
}

function through(offset: number, transition: Transition): number | null {
  if (transition.moved && transition.moved.from === offset) return transition.moved.to;
  return mapOffset(offset, transition.splices);
}

interface RefTarget {
  file: string;
  offset: number;
  rev: number;
}

interface Located {
  file: string;
  offset: number;
  el: MarkupElement;
}

function nodeParts(node: string): { file: string; offset: number } | null {
  const at = node.lastIndexOf("#");
  if (at <= 0 || at === node.length - 1) return null;
  const digits = node.slice(at + 1);
  if (!/^(0|[1-9][0-9]*)$/.test(digits)) return null;
  return { file: node.slice(0, at), offset: Number(digits) };
}

function nodeId(file: string, offset: number): string {
  return `${file}#${offset}`;
}

export function operationKeys(o: Operation): string[] {
  const keys: string[] = [];
  const add = (prefix: string, value: string | undefined) => {
    if (value) keys.push(prefix + value);
  };
  const nodeKey = (node: string | undefined) => {
    if (!node) return;
    if (node.startsWith("@")) return add("ref:", node.slice(1));
    const parts = nodeParts(node);
    if (parts) add("file:", parts.file);
  };
  switch (o.op) {
    case "write_file":
    case "edit_file":
    case "delete_file":
    case "rename_file":
      add("file:", o.path);
      add("file:", o.to);
      break;
    case "set_token":
      add("token:", o.name);
      break;
    case "set_knob":
      add("knob:", o.name);
      add("file:", knobsPath);
      break;
    default:
      nodeKey(o.node);
      nodeKey(o.before);
      nodeKey(o.after);
      nodeKey(o.at?.before);
      nodeKey(o.at?.after);
      nodeKey(o.at?.into);
      add("file:", o.at?.page);
      add("ref:", o.ref);
  }
  return keys;
}

class Batch implements Workspace {
  readonly stack: Stack;
  private readonly histories = new Map<string, History>();
  private readonly refs = new Map<string, RefTarget>();
  private readonly htmlCache = new Map<string, HtmlDocument>();
  private readonly jsxCache = new Map<string, JsxDocument>();

  constructor(project: Project, private readonly request: ExecuteRequest) {
    this.stack = project.stack;
    for (const [path, file] of Object.entries(project.files)) {
      this.histories.set(path, { versions: [{ rev: file.rev, content: file.content }], transitions: [] });
    }
  }

  content(path: string): string | undefined {
    const history = this.histories.get(path);
    return history ? history.versions[history.versions.length - 1].content : undefined;
  }

  rev(path: string): number {
    const history = this.histories.get(path);
    return history ? history.versions[history.versions.length - 1].rev : 0;
  }

  paths(): string[] {
    return [...this.histories.keys()].sort();
  }

  files(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const path of this.paths()) out[path] = this.content(path)!;
    return out;
  }

  html(path: string): HtmlDocument {
    return this.htmlAt(path, this.content(path)!);
  }

  jsx(path: string): JsxDocument {
    return this.jsxAt(path, this.content(path)!);
  }

  private htmlAt(path: string, content: string): HtmlDocument {
    let doc = this.htmlCache.get(content);
    if (!doc) {
      doc = parseHtml(content);
      this.htmlCache.set(content, doc);
    }
    return doc;
  }

  private jsxAt(path: string, content: string): JsxDocument {
    const key = `${path}\u0000${content}`;
    let doc = this.jsxCache.get(key);
    if (!doc) {
      doc = parseJsx(content, path);
      this.jsxCache.set(key, doc);
    }
    return doc;
  }

  adapter(path: string): MarkupAdapter {
    const language = markupLanguage(this.stack, path);
    if (language === "html") return htmlAdapter;
    if (language === "jsx") return jsxAdapter;
    throw new OpFailure("node_invalid", "node");
  }

  private elementIn(path: string, content: string, offset: number): MarkupElement | undefined {
    const language = markupLanguage(this.stack, path);
    try {
      if (language === "html") return this.htmlAt(path, content).at(offset);
      if (language === "jsx") return this.jsxAt(path, content).at(offset);
    } catch {
      return undefined;
    }
    return undefined;
  }

  private mapForward(path: string, offset: number, rev: number): number {
    const history = this.histories.get(path);
    if (!history) throw new OpFailure("path_not_found", "node");
    let index = history.versions.findIndex((version) => version.rev === rev);
    let current = offset;
    let originTag: string | undefined;
    if (index < 0) {
      const first = history.versions[0];
      const base = rev < first.rev ? this.request.bases[path]?.find((candidate) => candidate.rev === rev) : undefined;
      if (!base) throw new OpFailure("node_stale", "node");
      originTag = this.elementIn(path, base.content, offset)?.tag;
      if (!originTag) throw new OpFailure("node_stale", "node");
      const mapped = mapOffset(current, diffSplices(base.content, first.content));
      if (mapped === null) throw new OpFailure("node_stale", "node");
      current = mapped;
      index = 0;
    } else {
      originTag = this.elementIn(path, history.versions[index].content, offset)?.tag;
      if (!originTag) throw new OpFailure("node_stale", "node");
    }
    for (let i = index; i < history.transitions.length; i++) {
      const mapped = through(current, history.transitions[i]);
      if (mapped === null) throw new OpFailure("node_stale", "node");
      current = mapped;
    }
    const el = this.elementIn(path, this.content(path)!, current);
    if (!el || el.tag !== originTag) throw new OpFailure("node_stale", "node");
    return current;
  }

  locate(node: string | undefined, rev: number | undefined, field: string): Located {
    if (!node) throw new OpFailure("field_required", field);
    let file: string;
    let offset: number;
    if (node.startsWith("@")) {
      const target = this.refs.get(node.slice(1));
      if (!target) throw new OpFailure("ref_unknown", field);
      file = target.file;
      offset = this.mapForwardField(target.file, target.offset, target.rev, field);
    } else {
      const parts = nodeParts(node);
      if (!parts || markupLanguage(this.stack, parts.file) === null) throw new OpFailure("node_invalid", field);
      if (!this.histories.has(parts.file)) throw new OpFailure("path_not_found", field);
      file = parts.file;
      offset = this.mapForwardField(parts.file, parts.offset, rev ?? this.rev(parts.file), field);
    }
    const el = this.adapter(file).element(this, file, offset);
    if (!el) throw new OpFailure("node_stale", field);
    return { file, offset, el };
  }

  private mapForwardField(path: string, offset: number, rev: number, field: string): number {
    try {
      return this.mapForward(path, offset, rev);
    } catch (error) {
      if (error instanceof OpFailure) throw new OpFailure(error.code, field, error.detail);
      throw error;
    }
  }

  apply(change: OpChange, ref: string | undefined) {
    for (const rename of change.renamed ?? []) {
      const history = this.histories.get(rename.from)!;
      this.histories.delete(rename.from);
      const last = history.versions[history.versions.length - 1];
      history.versions.push({ rev: last.rev + 1, content: last.content });
      history.transitions.push({ splices: [] });
      this.histories.set(rename.to, history);
    }
    for (const path of change.deleted ?? []) this.histories.delete(path);
    for (const [path, file] of Object.entries(change.files)) {
      const history = this.histories.get(path);
      if (!history) {
        this.histories.set(path, { versions: [{ rev: 1, content: file.content }], transitions: [] });
        continue;
      }
      const last = history.versions[history.versions.length - 1];
      if (last.content === file.content) continue;
      history.versions.push({ rev: last.rev + 1, content: file.content });
      history.transitions.push({ splices: file.splices, moved: file.moved });
    }
    if (ref && change.created) this.refs.set(ref, { file: change.created.file, offset: change.created.offset, rev: this.rev(change.created.file) });
  }

  runs(index: number) {
    const runs = this.request.texts[String(index)];
    if (!Array.isArray(runs)) throw new OpFailure("field_invalid", "text");
    return runs;
  }

  mediaURL(id: string | undefined): string {
    const url = id ? this.request.media[id] : undefined;
    if (!url) throw new OpFailure("media_invalid", "media_id");
    return url;
  }

  section(o: Operation): SectionSource {
    if (o.html && o.html.trim() !== "") return { markup: o.html, language: "html" };
    const jsx = o.library_key ? this.request.jsx_sections?.[o.library_key] : undefined;
    if (jsx) return { markup: jsx, language: "jsx" };
    const html = o.library_key ? this.request.sections[o.library_key] : undefined;
    if (!html) throw new OpFailure("library_key_invalid", "library_key");
    return { markup: html, language: "html" };
  }
}

function single(path: string, patcher: Patcher): Record<string, FileChange> {
  return { [path]: patcher.result() };
}

function mappedFocus(file: string, offset: number, files: Record<string, FileChange>): string {
  const change = files[file];
  if (!change) return nodeId(file, offset);
  const mapped = mapOffset(offset, change.splices);
  return mapped === null ? file : nodeId(file, mapped);
}

async function visual(batch: Batch, o: Operation, index: number): Promise<OpChange> {
  switch (o.op) {
    case "set_text": {
      const target = batch.locate(o.node, o.rev, "node");
      const files = batch.adapter(target.file).setText(batch, target.file, target.el, batch.runs(index), o.item);
      return { files, focus: mappedFocus(target.file, target.offset, files) };
    }
    case "set_attr": {
      const target = batch.locate(o.node, o.rev, "node");
      const content = batch.content(target.file)!;
      const patcher = new Patcher(content);
      batch.adapter(target.file).setAttr(patcher, content, target.el, o.name!, typeof o.value === "string" ? o.value : null);
      return { files: single(target.file, patcher), focus: nodeId(target.file, target.offset) };
    }
    case "set_classes":
      return setClasses(batch, o);
    case "replace_image": {
      const target = batch.locate(o.node, o.rev, "node");
      const adapter = batch.adapter(target.file);
      if (!adapter.isImage(target.el)) throw new OpFailure("node_invalid", "node");
      const url = batch.mediaURL(o.media_id);
      const content = batch.content(target.file)!;
      const patcher = new Patcher(content);
      adapter.setAttr(patcher, content, target.el, "src", url);
      if (o.alt !== undefined) adapter.setAttr(patcher, content, target.el, "alt", o.alt);
      adapter.setAttr(patcher, content, target.el, "srcset", null);
      return { files: single(target.file, patcher), focus: nodeId(target.file, target.offset) };
    }
    case "move":
      return move(batch, o);
    case "duplicate":
      return duplicate(batch, o);
    case "remove":
      return remove(batch, o);
    case "insert_section":
      return insertSection(batch, o);
  }
  throw new OpFailure("operation_unknown", "op");
}

async function setClasses(batch: Batch, o: Operation): Promise<OpChange> {
  const target = batch.locate(o.node, o.rev, "node");
  const content = batch.content(target.file)!;
  const list = batch.adapter(target.file).classes(content, target.el);
  const prefix = o.breakpoint ? `${o.breakpoint}:` : "";
  const adds = (o.add ?? []).map((cls) => prefix + cls);
  const removes = new Set((o.remove ?? []).map((cls) => prefix + cls));
  let merge: ClassMerger | null = null;
  if (adds.length > 0) {
    const files = batch.files();
    const styles = styleFiles(batch.stack, files);
    const entry = tailwindEntry(batch.stack, files);
    const valid = await classValidator(entry, styles);
    if (adds.some((cls) => !valid(cls))) throw new OpFailure("class_invalid", "add");
    merge = await classMerger(entry, styles);
  }
  const addition = adds.join(" ");
  const kept = list.tokens.filter((token) => !removes.has(token) && (merge === null || merge(`${token} ${addition}`).split(" ").includes(token)));
  const next = [...kept];
  for (const cls of adds) if (!next.includes(cls)) next.push(cls);
  const patcher = new Patcher(content);
  if (next.join(" ") !== list.tokens.join(" ")) list.write(patcher, next);
  return { files: single(target.file, patcher), focus: nodeId(target.file, target.offset) };
}

function requirePlaceable(el: MarkupElement, field: string) {
  if (!el.placeable) throw new OpFailure("position_invalid", field);
}

async function move(batch: Batch, o: Operation): Promise<OpChange> {
  const target = batch.locate(o.node, o.rev, "node");
  const anchorField = o.before ? "before" : "after";
  const anchor = batch.locate(o.before ?? o.after, o.rev, anchorField);
  if (anchor.file !== target.file || contains(target.el, anchor.el) || contains(anchor.el, target.el)) throw new OpFailure("position_invalid", anchorField);
  requirePlaceable(target.el, "node");
  requirePlaceable(anchor.el, anchorField);
  const content = batch.content(target.file)!;
  const cut = lineBlock(content, target.el.start, target.el.end);
  const at = o.before ? beforeInsertion(content, anchor.el) : afterInsertion(content, anchor.el);
  const placed = placeText(content, at, content.slice(target.el.start, target.el.end), cut.indent);
  const patcher = new Patcher(content).remove(cut.start, cut.end);
  if (o.before) patcher.insertBefore(at.index, placed.text);
  else patcher.insertAfter(at.index, placed.text);
  const shifted = at.index >= cut.end ? at.index - (cut.end - cut.start) : at.index;
  const offset = shifted + placed.offset;
  const files = single(target.file, patcher);
  files[target.file].moved = { from: target.el.start, to: offset };
  return { files, focus: nodeId(target.file, offset), created: { file: target.file, offset } };
}

async function duplicate(batch: Batch, o: Operation): Promise<OpChange> {
  const target = batch.locate(o.node, o.rev, "node");
  requirePlaceable(target.el, "node");
  const content = batch.content(target.file)!;
  const block = lineBlock(content, target.el.start, target.el.end);
  const at = afterInsertion(content, target.el);
  const placed = placeText(content, at, content.slice(target.el.start, target.el.end), block.indent);
  const patcher = new Patcher(content).insertAfter(at.index, placed.text);
  const offset = at.index + placed.offset;
  return { files: single(target.file, patcher), focus: nodeId(target.file, offset), created: { file: target.file, offset } };
}

async function remove(batch: Batch, o: Operation): Promise<OpChange> {
  const target = batch.locate(o.node, o.rev, "node");
  requirePlaceable(target.el, "node");
  const content = batch.content(target.file)!;
  const block = lineBlock(content, target.el.start, target.el.end);
  const files = single(target.file, new Patcher(content).remove(block.start, block.end));
  const parent = target.el.parent;
  return { files, focus: parent ? mappedFocus(target.file, parent.start, files) : target.file };
}

function parsesAsJsx(source: string): boolean {
  try {
    parseJsx(`const section = (\n<>\n${source}\n</>\n);\n`, "section.jsx");
    return true;
  } catch {
    return false;
  }
}

function sectionMarkup(batch: Batch, file: string, section: SectionSource): string {
  if (section.language === "html") return batch.adapter(file).section(section.markup);
  if (markupLanguage(batch.stack, file) !== "jsx" || !parsesAsJsx(section.markup)) throw new OpFailure("library_key_invalid", "library_key");
  return section.markup.trim();
}

async function insertSection(batch: Batch, o: Operation): Promise<OpChange> {
  const placement = o.at ?? {};
  const section = batch.section(o);
  let file: string;
  let at;
  if (placement.page) {
    file = placement.page;
    if (batch.content(file) === undefined) throw new OpFailure("path_not_found", "at");
    at = batch.adapter(file).pageInsertion(batch, file);
    if (!at) throw new OpFailure("position_invalid", "at");
  } else {
    const anchor = batch.locate(placement.before ?? placement.after ?? placement.into, o.rev, "at");
    file = anchor.file;
    const content = batch.content(file)!;
    if (placement.into) {
      if (anchor.el.innerEnd === null) throw new OpFailure("position_invalid", "at");
      at = intoInsertion(content, anchor.el.innerEnd);
    } else {
      requirePlaceable(anchor.el, "at");
      at = placement.before ? beforeInsertion(content, anchor.el) : afterInsertion(content, anchor.el);
    }
  }
  const markup = sectionMarkup(batch, file, section);
  if (markup === "") throw new OpFailure("html_invalid", "html");
  const content = batch.content(file)!;
  const placed = placeText(content, at, markup);
  const patcher = new Patcher(content);
  if (placement.before) patcher.insertBefore(at.index, placed.text);
  else patcher.insertAfter(at.index, placed.text);
  const offset = at.index + placed.offset;
  return { files: single(file, patcher), focus: nodeId(file, offset), created: { file, offset } };
}

function theme(batch: Batch, o: Operation): OpChange {
  const files = batch.files();
  if (o.op === "set_token") {
    const path = themeFile(batch.stack, files) ?? tailwindEntry(batch.stack, files);
    if (!path) throw new OpFailure("path_not_found", "name");
    const edit = setThemeToken(files[path], o.name!, String(o.value));
    return { files: { [path]: edit }, focus: path };
  }
  const raw = files[knobsPath];
  if (raw === undefined) throw new OpFailure("knob_unknown", "name");
  let knobs: Array<{ name: string; variable: string; kind: string; unit?: string }>;
  try {
    knobs = JSON.parse(raw).knobs;
  } catch {
    throw new OpFailure("knob_unknown", "name");
  }
  const knob = Array.isArray(knobs) ? knobs.find((candidate) => candidate.name === o.name) : undefined;
  if (!knob) throw new OpFailure("knob_unknown", "name");
  let value: string;
  if (knob.kind === "range" && typeof o.value === "number") value = `${o.value}${knob.unit ?? ""}`;
  else if (knob.kind === "color" && typeof o.value === "string") value = o.value;
  else throw new OpFailure("knob_value_invalid", "value");
  const styles = styleFiles(batch.stack, files);
  const preferred = themeFile(batch.stack, files);
  const order = [...(preferred ? [preferred] : []), ...Object.keys(styles).sort().filter((path) => path !== preferred)];
  for (const path of order) {
    const edit = setCssVariable(styles[path], knob.variable, value);
    if (edit) return { files: { [path]: edit }, focus: path };
  }
  throw new OpFailure("knob_unknown", "name");
}

function code(batch: Batch, o: Operation): OpChange {
  const path = o.path!;
  switch (o.op) {
    case "write_file": {
      const before = batch.content(path);
      const content = o.content ?? "";
      return { files: { [path]: { content, splices: before === undefined ? [] : diffSplices(before, content) } }, focus: path };
    }
    case "edit_file": {
      const content = batch.content(path);
      if (content === undefined) throw new OpFailure("path_not_found", "path");
      const found = locateEdit(content, o.old ?? "", Boolean(o.replace_all));
      if (!found.ok) throw new OpFailure(found.code, "old", found.detail);
      const patcher = new Patcher(content);
      for (const [start, end] of found.ranges) patcher.replace(start, end, o.new ?? "");
      return { files: single(path, patcher), focus: path };
    }
    case "delete_file":
      if (batch.content(path) === undefined) throw new OpFailure("path_not_found", "path");
      return { files: {}, deleted: [path], focus: path };
    case "rename_file":
      if (batch.content(path) === undefined) throw new OpFailure("path_not_found", "path");
      if (batch.content(o.to!) !== undefined) throw new OpFailure("path_exists", "to");
      return { files: {}, renamed: [{ from: path, to: o.to! }], focus: o.to };
  }
  throw new OpFailure("operation_unknown", "op");
}

async function run(batch: Batch, o: Operation, index: number): Promise<OpChange> {
  switch (o.op) {
    case "write_file":
    case "edit_file":
    case "delete_file":
    case "rename_file":
      return code(batch, o);
    case "set_token":
    case "set_knob":
      return theme(batch, o);
  }
  return visual(batch, o, index);
}

function toStep(o: Operation, index: number, change: OpChange): StepPatch {
  const step: StepPatch = { op: o.op, index };
  if (change.focus) step.focus = change.focus;
  const files = Object.keys(change.files);
  if (files.length > 0) {
    step.files = {};
    for (const path of files.sort()) step.files[path] = change.files[path].content;
  }
  if (change.deleted?.length) step.deleted = change.deleted;
  if (change.renamed?.length) step.renamed = change.renamed;
  return step;
}

export const operationFailed = "operation_failed";

function unexpected(o: Operation, error: unknown): OpFailure {
  console.error(`sitebuilder operation ${o.op} failed`, error);
  return new OpFailure(operationFailed);
}

export async function execute(request: ExecuteRequest): Promise<ExecuteResult> {
  const batch = new Batch(request.project, request);
  const steps: StepPatch[] = [];
  const splices: StepSplice[] = [];
  const failures: Failure[] = [];
  const skipped: Skip[] = [];
  const poisoned = new Map<string, number>();
  for (let index = 0; index < request.operations.length; index++) {
    const o = request.operations[index];
    const keys = operationKeys(o);
    const root = keys.map((key) => poisoned.get(key)).find((value) => value !== undefined);
    if (root !== undefined) {
      skipped.push({ index, after: root });
      for (const key of keys) if (!poisoned.has(key)) poisoned.set(key, root);
      continue;
    }
    try {
      const change = await run(batch, o, index);
      const changed: Record<string, FileChange> = {};
      for (const [path, file] of Object.entries(change.files)) {
        if (batch.content(path) !== file.content || (change.renamed ?? []).some((r) => r.to === path)) changed[path] = file;
      }
      const effective = { ...change, files: changed };
      steps.push(toStep(o, index, effective));
      for (const [path, file] of Object.entries(changed)) for (const s of file.splices) splices.push({ index, path, ...s });
      batch.apply(effective, o.ref);
    } catch (caught) {
      const error = caught instanceof OpFailure ? caught : unexpected(o, caught);
      const failure: Failure = { index, code: error.code };
      if (error.field) failure.field = error.field;
      if (error.detail) failure.detail = error.detail;
      failures.push(failure);
      for (const key of keys) if (!poisoned.has(key)) poisoned.set(key, index);
    }
  }
  if (failures.length > 0) return skipped.length > 0 ? { failures, skipped } : { failures };
  return { steps, splices };
}

export function extract(request: ExtractRequest): ExtractResult {
  const batch = new Batch(request.project, { project: request.project, operations: [], texts: {}, media: {}, sections: {}, bases: request.bases });
  try {
    const target = batch.locate(request.node, request.rev, "node");
    return { source: batch.content(target.file)!.slice(target.el.start, target.el.end) };
  } catch (error) {
    if (!(error instanceof OpFailure)) throw error;
    const failure: Omit<Failure, "index"> = { code: error.code };
    if (error.field) failure.field = error.field;
    if (error.detail) failure.detail = error.detail;
    return { failure };
  }
}

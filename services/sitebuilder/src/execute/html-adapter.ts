import { attribute, trimmedRange, type MarkupAttr, type MarkupElement } from "../markup/element";
import { inlineOnly, type HtmlElement } from "../markup/html";
import type { ClassList, Insertion, MarkupAdapter, Workspace } from "./adapter";
import { escapeHtmlAttribute, htmlAttributeValue, renderRuns, type Run } from "./inline";
import { Patcher } from "./patcher";
import { OpFailure, type FileChange } from "./types";

function precedingSpace(content: string, start: number): number {
  let s = start;
  while (s > 0 && /[ \t\r\n]/.test(content[s - 1])) s--;
  return s;
}

function attributeInsertPoint(el: MarkupElement): number {
  return el.attrs.length > 0 ? el.attrs[el.attrs.length - 1].end : el.nameEnd;
}

function quoted(content: string, attr: MarkupAttr): boolean {
  const before = content[attr.valueStart - 1];
  return attr.value !== null && (before === '"' || before === "'");
}

export function removeAttribute(patcher: Patcher, content: string, attr: MarkupAttr) {
  patcher.remove(precedingSpace(content, attr.start), attr.end);
}

export const htmlAdapter: MarkupAdapter = {
  element(ws: Workspace, path: string, offset: number) {
    return ws.html(path).at(offset);
  },

  setText(ws: Workspace, path: string, el: MarkupElement, runs: Run[], item: number | undefined): Record<string, FileChange> {
    if (item !== undefined) throw new OpFailure("field_invalid", "item");
    if (el.innerStart === null || el.innerEnd === null) throw new OpFailure("node_invalid", "node");
    if (!inlineOnly(el as HtmlElement)) throw new OpFailure("node_invalid", "node");
    const content = ws.content(path)!;
    const [start, end] = trimmedRange(content, el.innerStart, el.innerEnd);
    const patcher = new Patcher(content).replace(start, end, renderRuns(runs, "html"));
    return { [path]: patcher.result() };
  },

  setAttr(patcher: Patcher, content: string, el: MarkupElement, name: string, value: string | null) {
    const existing = attribute(el, name);
    if (value === null) {
      if (existing) removeAttribute(patcher, content, existing);
      return;
    }
    if (existing && quoted(content, existing)) {
      const quote = content[existing.valueStart - 1];
      const escaped = quote === "'" ? value.replace(/&/g, "&amp;").replace(/'/g, "&#39;") : escapeHtmlAttribute(value);
      patcher.replace(existing.valueStart, existing.valueEnd, escaped);
      return;
    }
    if (existing) {
      patcher.replace(existing.start, existing.end, `${name}=${htmlAttributeValue(value)}`);
      return;
    }
    patcher.insertAfter(attributeInsertPoint(el), ` ${name}=${htmlAttributeValue(value)}`);
  },

  classes(content: string, el: MarkupElement): ClassList {
    const existing = attribute(el, "class");
    const tokens = existing?.value ? existing.value.split(/\s+/).filter(Boolean) : [];
    return {
      tokens,
      write(patcher: Patcher, next: string[]) {
        const value = next.join(" ");
        if (value === "") {
          if (existing) removeAttribute(patcher, content, existing);
          return;
        }
        if (existing && quoted(content, existing)) {
          patcher.replace(existing.valueStart, existing.valueEnd, value);
          return;
        }
        if (existing) {
          patcher.replace(existing.start, existing.end, `class="${value}"`);
          return;
        }
        patcher.insertAfter(el.nameEnd, ` class="${value}"`);
      },
    };
  },

  isImage(el: MarkupElement) {
    return el.tag === "img";
  },

  section(html: string) {
    return html.trim();
  },

  pageInsertion(ws: Workspace, path: string): Insertion | null {
    const content = ws.content(path)!;
    return { index: content.length, indent: "", ownLine: true };
  },
};

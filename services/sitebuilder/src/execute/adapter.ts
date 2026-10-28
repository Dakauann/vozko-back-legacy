import type { MarkupElement } from "../markup/element";
import type { HtmlDocument } from "../markup/html";
import type { JsxDocument } from "../markup/jsx";
import type { Stack } from "../project";
import type { Run } from "./inline";
import type { Patcher } from "./patcher";
import type { FileChange } from "./types";

export interface Workspace {
  stack: Stack;
  content(path: string): string | undefined;
  paths(): string[];
  html(path: string): HtmlDocument;
  jsx(path: string): JsxDocument;
}

export interface ClassList {
  tokens: string[];
  write(patcher: Patcher, tokens: string[]): void;
}

export interface Insertion {
  index: number;
  indent: string;
  ownLine: boolean;
}

export interface MarkupAdapter {
  element(ws: Workspace, path: string, offset: number): MarkupElement | undefined;
  setText(ws: Workspace, path: string, el: MarkupElement, runs: Run[], item: number | undefined): Record<string, FileChange>;
  setAttr(patcher: Patcher, content: string, el: MarkupElement, name: string, value: string | null): void;
  classes(content: string, el: MarkupElement): ClassList;
  isImage(el: MarkupElement): boolean;
  section(html: string): string;
  pageInsertion(ws: Workspace, path: string): Insertion | null;
}

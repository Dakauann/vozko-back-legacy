import { lineBlock, reindent, type MarkupElement } from "../markup/element";
import type { Insertion } from "./adapter";

const indentUnit = "  ";

export function intoInsertion(content: string, innerEnd: number): Insertion {
  let lineStart = innerEnd;
  while (lineStart > 0 && content[lineStart - 1] !== "\n") lineStart--;
  if (lineStart > 0 && /^[ \t]*$/.test(content.slice(lineStart, innerEnd))) {
    return { index: lineStart, indent: content.slice(lineStart, innerEnd) + indentUnit, ownLine: true };
  }
  return { index: innerEnd, indent: "", ownLine: false };
}

export function beforeInsertion(content: string, anchor: MarkupElement): Insertion {
  const block = lineBlock(content, anchor.start, anchor.end);
  return block.whole ? { index: block.start, indent: block.indent, ownLine: true } : { index: anchor.start, indent: "", ownLine: false };
}

export function afterInsertion(content: string, anchor: MarkupElement): Insertion {
  const block = lineBlock(content, anchor.start, anchor.end);
  return block.whole ? { index: block.end, indent: block.indent, ownLine: true } : { index: anchor.end, indent: "", ownLine: false };
}

export interface Placed {
  text: string;
  offset: number;
}

export function placeText(content: string, at: Insertion, markup: string, sourceIndent = ""): Placed {
  const trimmed = markup.trim();
  if (!at.ownLine) return { text: trimmed, offset: 0 };
  const body = trimmed
    .split("\n")
    .map((line, i) => (i === 0 || line.trim() === "" ? line : reindent(line, sourceIndent, "")))
    .map((line) => (line.trim() === "" ? "" : at.indent + line))
    .join("\n");
  const needsNewline = at.index > 0 && content[at.index - 1] !== "\n";
  const prefix = needsNewline ? "\n" : "";
  return { text: `${prefix}${body}\n`, offset: prefix.length + at.indent.length };
}

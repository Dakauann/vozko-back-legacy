export interface MarkupAttr {
  name: string;
  start: number;
  end: number;
  value: string | null;
  valueStart: number;
  valueEnd: number;
}

export interface MarkupElement {
  tag: string;
  start: number;
  end: number;
  nameEnd: number;
  startTagEnd: number;
  innerStart: number | null;
  innerEnd: number | null;
  attrs: MarkupAttr[];
  parent: MarkupElement | null;
  children: MarkupElement[];
  component: boolean;
  placeable: boolean;
}

export const inlineTags = new Set(["b", "strong", "i", "em", "a", "br", "span", "small", "mark", "code", "sup", "sub", "u", "s"]);

export function contains(outer: MarkupElement, inner: MarkupElement): boolean {
  return outer.start <= inner.start && inner.end <= outer.end;
}

export function attribute(el: MarkupElement, name: string): MarkupAttr | undefined {
  return el.attrs.find((attr) => attr.name === name);
}

export function trimmedRange(content: string, start: number, end: number): [number, number] {
  let s = start;
  let e = end;
  while (s < e && /\s/.test(content[s])) s++;
  while (e > s && /\s/.test(content[e - 1])) e--;
  if (s === e) return [start, end];
  return [s, e];
}

export interface LineBlock {
  start: number;
  end: number;
  whole: boolean;
  indent: string;
}

export function lineBlock(content: string, start: number, end: number): LineBlock {
  let lineStart = start;
  while (lineStart > 0 && content[lineStart - 1] !== "\n") lineStart--;
  let lineEnd = end;
  while (lineEnd < content.length && content[lineEnd] !== "\n") lineEnd++;
  const before = content.slice(lineStart, start);
  const after = content.slice(end, lineEnd);
  const whole = /^[ \t]*$/.test(before) && /^[ \t\r]*$/.test(after);
  if (!whole) return { start, end, whole: false, indent: "" };
  return { start: lineStart, end: lineEnd < content.length ? lineEnd + 1 : lineEnd, whole: true, indent: before };
}

export function reindent(text: string, from: string, to: string): string {
  if (from === to) return text;
  return text
    .split("\n")
    .map((line) => (line.startsWith(from) ? to + line.slice(from.length) : line))
    .join("\n");
}

export function lineIndentAt(content: string, offset: number): string {
  let lineStart = offset;
  while (lineStart > 0 && content[lineStart - 1] !== "\n") lineStart--;
  const match = /^[ \t]*/.exec(content.slice(lineStart));
  return match ? match[0] : "";
}

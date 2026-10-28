import { parseFragment, type DefaultTreeAdapterMap } from "parse5";
import { inlineTags, type MarkupAttr, type MarkupElement } from "./element";

type Node = DefaultTreeAdapterMap["node"];
type Element = DefaultTreeAdapterMap["element"];
type Template = DefaultTreeAdapterMap["template"];

const voidTags = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"]);

export interface HtmlElement extends MarkupElement {
  node: Element;
}

export interface HtmlDocument {
  content: string;
  elements: HtmlElement[];
  at(offset: number): HtmlElement | undefined;
}

function isElement(node: Node): node is Element {
  return "tagName" in node;
}

function childNodesOf(node: Node): Node[] {
  if (isElement(node) && node.tagName === "template") return (node as Template).content.childNodes as Node[];
  return "childNodes" in node ? (node.childNodes as Node[]) : [];
}

export function scanAttribute(content: string, start: number, end: number): MarkupAttr {
  const raw = content.slice(start, end);
  const eq = raw.indexOf("=");
  if (eq < 0) return { name: raw.trim().toLowerCase(), start, end, value: null, valueStart: end, valueEnd: end };
  const name = raw.slice(0, eq).trim().toLowerCase();
  let i = eq + 1;
  while (i < raw.length && /\s/.test(raw[i])) i++;
  const quote = raw[i];
  if (quote === '"' || quote === "'") {
    const close = raw.indexOf(quote, i + 1);
    const valueEnd = close < 0 ? raw.length : close;
    return { name, start, end, value: raw.slice(i + 1, valueEnd), valueStart: start + i + 1, valueEnd: start + valueEnd };
  }
  return { name, start, end, value: raw.slice(i), valueStart: start + i, valueEnd: end };
}

export function parseHtml(content: string): HtmlDocument {
  const fragment = parseFragment(content, { sourceCodeLocationInfo: true });
  const elements: HtmlElement[] = [];
  const byStart = new Map<number, HtmlElement>();
  const visit = (node: Node, parent: HtmlElement | null) => {
    for (const child of childNodesOf(node)) {
      if (!isElement(child)) continue;
      const loc = child.sourceCodeLocation;
      if (!loc || !loc.startTag) {
        visit(child, parent);
        continue;
      }
      const tag = child.tagName;
      const attrs: MarkupAttr[] = [];
      const attrLocations = loc.startTag.attrs ?? {};
      for (const key of Object.keys(attrLocations)) {
        const at = attrLocations[key];
        attrs.push(scanAttribute(content, at.startOffset, at.endOffset));
      }
      attrs.sort((a, b) => a.start - b.start);
      const selfClosing = content.slice(loc.startTag.startOffset, loc.startTag.endOffset).endsWith("/>");
      const hasEnd = Boolean(loc.endTag) && !voidTags.has(tag);
      const element: HtmlElement = {
        tag,
        start: loc.startOffset,
        end: loc.endTag ? loc.endTag.endOffset : loc.startTag.endOffset,
        nameEnd: loc.startTag.startOffset + 1 + tag.length,
        startTagEnd: loc.startTag.endOffset,
        innerStart: hasEnd ? loc.startTag.endOffset : null,
        innerEnd: hasEnd && loc.endTag ? loc.endTag.startOffset : null,
        attrs,
        parent,
        children: [],
        component: false,
        placeable: voidTags.has(tag) || selfClosing || Boolean(loc.endTag),
        node: child,
      };
      elements.push(element);
      byStart.set(element.start, element);
      if (parent) parent.children.push(element);
      visit(child, element);
    }
  };
  visit(fragment as Node, null);
  return { content, elements, at: (offset) => byStart.get(offset) };
}

export function inlineOnly(el: HtmlElement): boolean {
  const check = (node: Node): boolean => {
    for (const child of childNodesOf(node)) {
      if (!isElement(child)) continue;
      if (!inlineTags.has(child.tagName) || !check(child)) return false;
    }
    return true;
  };
  return check(el.node);
}

export function isVoidTag(tag: string): boolean {
  return voidTags.has(tag);
}

export function topLevelHtmlElements(content: string): HtmlElement[] {
  return parseHtml(content).elements.filter((el) => el.parent === null);
}

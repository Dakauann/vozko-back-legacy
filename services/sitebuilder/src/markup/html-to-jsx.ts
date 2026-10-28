import { parseFragment, type DefaultTreeAdapterMap } from "parse5";
import { escapeJsxText, jsxAttributeValue } from "../execute/inline";
import { reactAttributeName } from "./jsx-attrs";
import { isVoidTag } from "./html";

type Node = DefaultTreeAdapterMap["node"];
type Element = DefaultTreeAdapterMap["element"];
type Template = DefaultTreeAdapterMap["template"];

function styleObject(value: string): string {
  const entries: string[] = [];
  for (const declaration of value.split(";")) {
    const colon = declaration.indexOf(":");
    if (colon < 0) continue;
    const property = declaration.slice(0, colon).trim();
    const raw = declaration.slice(colon + 1).trim();
    if (!property || !raw) continue;
    const key = property.startsWith("--") ? JSON.stringify(property) : property.replace(/^-ms-/, "ms-").replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
    entries.push(`${key}: ${JSON.stringify(raw)}`);
  }
  return `{{ ${entries.join(", ")} }}`;
}

function attributes(element: Element): string {
  let out = "";
  for (const attr of element.attrs) {
    const name = attr.prefix ? `${attr.prefix}:${attr.name}` : attr.name;
    if (/^on/i.test(name)) continue;
    const react = reactAttributeName(name);
    if (react === "style") {
      out += ` style=${styleObject(attr.value)}`;
      continue;
    }
    out += attr.value === "" && !react.startsWith("data-") && !react.startsWith("aria-") && react !== "alt" && react !== "value" ? ` ${react}` : ` ${react}=${jsxAttributeValue(attr.value)}`;
  }
  return out;
}

function childrenOf(node: Node): Node[] {
  if ("tagName" in node && node.tagName === "template") return (node as Template).content.childNodes as Node[];
  return "childNodes" in node ? (node.childNodes as Node[]) : [];
}

function serialize(node: Node): string {
  if (node.nodeName === "#text") return escapeJsxText((node as DefaultTreeAdapterMap["textNode"]).value);
  if (!("tagName" in node)) return "";
  const element = node as Element;
  const inner = childrenOf(element).map(serialize).join("");
  if (isVoidTag(element.tagName) || (inner === "" && element.namespaceURI !== "http://www.w3.org/1999/xhtml")) return `<${element.tagName}${attributes(element)} />`;
  return `<${element.tagName}${attributes(element)}>${inner}</${element.tagName}>`;
}

export function htmlToJsx(html: string): string {
  const fragment = parseFragment(html);
  return (fragment.childNodes as Node[]).map(serialize).join("").trim();
}

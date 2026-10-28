import MagicString from "magic-string";
import { parseFragment, serialize, type DefaultTreeAdapterMap } from "parse5";
import { enclosing, isFunctionNode, moduleConstant, parseJsx, unwrapExpression, type AstNode, type JsxDocument } from "../markup/jsx";

export const nodeAttr = "data-vz-node";
export const instanceAttr = "data-vz-inst";
export const itemAttr = "data-vz-i";
const itemParam = "__vzI";

function literalMap(doc: JsxDocument, fn: AstNode): boolean {
  const call = doc.parents.get(fn);
  if (!call || call.type !== "CallExpression" || call.arguments[0] !== fn) return false;
  const callee = call.callee as AstNode;
  if ((callee.type !== "MemberExpression" && callee.type !== "OptionalMemberExpression") || callee.computed || callee.property.name !== "map") return false;
  const source = unwrapExpression(callee.object);
  if (source?.type === "ArrayExpression") return true;
  if (source?.type === "Identifier") return moduleConstant(doc, source.name)?.type === "ArrayExpression";
  return false;
}

function indexName(content: string, fn: AstNode, magic: MagicString, added: Set<AstNode>): string | null {
  const params = fn.params as AstNode[];
  if (params.length >= 2) return params[1].type === "Identifier" ? params[1].name : null;
  if (params.length !== 1) return null;
  if (!added.has(fn)) {
    added.add(fn);
    const param = params[0];
    if (content.slice(fn.start, param.start).includes("(")) magic.appendLeft(param.end, `, ${itemParam}`);
    else {
      magic.prependRight(param.start, "(");
      magic.appendLeft(param.end, `, ${itemParam})`);
    }
  }
  return itemParam;
}

export function tagJsx(content: string, path: string): string {
  const doc = parseJsx(content, path);
  const magic = new MagicString(content);
  const added = new Set<AstNode>();
  for (const el of doc.elements) {
    const opening = el.node.openingElement as AstNode;
    if (opening.name.type === "JSXNamespacedName") continue;
    const id = `${path}#${el.start}`;
    const attr = el.component ? instanceAttr : nodeAttr;
    let extra = ` ${attr}="${id}"`;
    if (!el.component) {
      const fn = enclosing(doc, el.node, isFunctionNode);
      if (fn && literalMap(doc, fn)) {
        const name = indexName(content, fn, magic, added);
        if (name) extra += ` ${itemAttr}={${name}}`;
      }
    }
    magic.appendLeft(opening.name.end, extra);
  }
  return magic.toString();
}

type Node = DefaultTreeAdapterMap["node"];
type Element = DefaultTreeAdapterMap["element"];
type Template = DefaultTreeAdapterMap["template"];
export type Fragment = DefaultTreeAdapterMap["documentFragment"];

function childrenOf(node: Node): Node[] {
  if ("tagName" in node && node.tagName === "template") return (node as Template).content.childNodes as Node[];
  return "childNodes" in node ? (node.childNodes as Node[]) : [];
}

export function parseHtmlFragment(content: string, located: boolean): Fragment {
  return parseFragment(content, { sourceCodeLocationInfo: located });
}

export function tagHtml(fragment: Fragment, path: string) {
  const visit = (node: Node) => {
    for (const child of childrenOf(node)) {
      if ("tagName" in child) {
        const el = child as Element;
        const loc = el.sourceCodeLocation;
        if (loc?.startTag) el.attrs.push({ name: nodeAttr, value: `${path}#${loc.startOffset}` });
      }
      visit(child);
    }
  };
  visit(fragment as Node);
}

export function walkElements(fragment: Node, visit: (el: Element, parent: Node) => void) {
  const step = (node: Node) => {
    for (const child of [...childrenOf(node)]) {
      if ("tagName" in child) visit(child as Element, node);
      step(child);
    }
  };
  step(fragment);
}

export function serializeFragment(fragment: Fragment): string {
  return serialize(fragment);
}

import { parse } from "@babel/parser";
import type { MarkupAttr, MarkupElement } from "./element";

export type AstNode = { type: string; start: number; end: number; [key: string]: any };

const skippedKeys = new Set(["loc", "start", "end", "range", "extra", "leadingComments", "trailingComments", "innerComments", "type", "comments", "tokens", "errors"]);

export function isNode(value: unknown): value is AstNode {
  return typeof value === "object" && value !== null && typeof (value as AstNode).type === "string" && typeof (value as AstNode).start === "number";
}

export function childNodes(node: AstNode): AstNode[] {
  const out: AstNode[] = [];
  for (const key of Object.keys(node)) {
    if (skippedKeys.has(key)) continue;
    const value = node[key];
    if (Array.isArray(value)) {
      for (const item of value) if (isNode(item)) out.push(item);
    } else if (isNode(value)) out.push(value);
  }
  return out;
}

export function walk(root: AstNode, visit: (node: AstNode, parent: AstNode | null) => void) {
  const stack: Array<[AstNode, AstNode | null]> = [[root, null]];
  while (stack.length > 0) {
    const [node, parent] = stack.pop()!;
    visit(node, parent);
    const children = childNodes(node);
    for (let i = children.length - 1; i >= 0; i--) stack.push([children[i], node]);
  }
}

export function parserPlugins(path: string): Array<"jsx" | "typescript"> {
  if (path.endsWith(".tsx")) return ["jsx", "typescript"];
  if (path.endsWith(".ts")) return ["typescript"];
  return ["jsx"];
}

export function parseModule(content: string, path: string): AstNode {
  return parse(content, { sourceType: "module", plugins: parserPlugins(path), errorRecovery: false }) as unknown as AstNode;
}

export function jsxName(name: AstNode): string {
  switch (name.type) {
    case "JSXIdentifier":
      return name.name;
    case "JSXMemberExpression":
      return `${jsxName(name.object)}.${jsxName(name.property)}`;
    case "JSXNamespacedName":
      return `${name.namespace.name}:${name.name.name}`;
  }
  return "";
}

export function isComponentName(name: string): boolean {
  if (name.includes(":")) return false;
  const last = name.slice(name.lastIndexOf(".") + 1);
  return /^[A-Z]/.test(name) || /^[A-Z]/.test(last);
}

export interface JsxAttr extends MarkupAttr {
  node: AstNode;
  literal: boolean;
}

export interface JsxElement extends MarkupElement {
  node: AstNode;
  attrs: JsxAttr[];
  parentNode: AstNode | null;
}

export interface JsxDocument {
  content: string;
  path: string;
  ast: AstNode;
  elements: JsxElement[];
  parents: Map<AstNode, AstNode | null>;
  at(offset: number): JsxElement | undefined;
  of(node: AstNode): JsxElement | undefined;
}

function attributesOf(opening: AstNode): JsxAttr[] {
  const out: JsxAttr[] = [];
  for (const attr of opening.attributes as AstNode[]) {
    if (attr.type !== "JSXAttribute") continue;
    const name = attr.name.type === "JSXNamespacedName" ? `${attr.name.namespace.name}:${attr.name.name.name}` : attr.name.name;
    const value = attr.value as AstNode | null;
    if (!value) {
      out.push({ name, start: attr.start, end: attr.end, value: null, valueStart: attr.end, valueEnd: attr.end, node: attr, literal: false });
      continue;
    }
    if (value.type === "StringLiteral") {
      out.push({ name, start: attr.start, end: attr.end, value: value.value, valueStart: value.start + 1, valueEnd: value.end - 1, node: attr, literal: true });
      continue;
    }
    out.push({ name, start: attr.start, end: attr.end, value: null, valueStart: value.start, valueEnd: value.end, node: attr, literal: false });
  }
  return out;
}

export function parseJsx(content: string, path: string): JsxDocument {
  const ast = parseModule(content, path);
  const parents = new Map<AstNode, AstNode | null>();
  const elements: JsxElement[] = [];
  const byStart = new Map<number, JsxElement>();
  const byNode = new Map<AstNode, JsxElement>();
  walk(ast, (node, parent) => {
    parents.set(node, parent);
  });
  walk(ast, (node, parentNode) => {
    if (node.type !== "JSXElement") return;
    const opening = node.openingElement as AstNode;
    const closing = node.closingElement as AstNode | null;
    const tag = jsxName(opening.name);
    let ancestor = parentNode;
    while (ancestor && ancestor.type !== "JSXElement") ancestor = parents.get(ancestor) ?? null;
    const parent = ancestor ? byNode.get(ancestor) ?? null : null;
    const element: JsxElement = {
      tag,
      start: node.start,
      end: node.end,
      nameEnd: opening.name.end,
      startTagEnd: opening.end,
      innerStart: closing ? opening.end : null,
      innerEnd: closing ? closing.start : null,
      attrs: attributesOf(opening),
      parent,
      children: [],
      component: isComponentName(tag),
      placeable: Boolean(parentNode && (parentNode.type === "JSXElement" || parentNode.type === "JSXFragment")),
      node,
      parentNode,
    };
    elements.push(element);
    byStart.set(element.start, element);
    byNode.set(node, element);
    if (parent) parent.children.push(element);
  });
  return { content, path, ast, elements, parents, at: (offset) => byStart.get(offset), of: (node) => byNode.get(node) };
}

export function enclosing(doc: JsxDocument, node: AstNode, match: (candidate: AstNode) => boolean): AstNode | null {
  let current = doc.parents.get(node) ?? null;
  while (current) {
    if (match(current)) return current;
    current = doc.parents.get(current) ?? null;
  }
  return null;
}

export function isFunctionNode(node: AstNode): boolean {
  return node.type === "ArrowFunctionExpression" || node.type === "FunctionExpression" || node.type === "FunctionDeclaration";
}

export function stringLiteralValue(node: AstNode | null | undefined): string | null {
  if (!node) return null;
  if (node.type === "StringLiteral") return node.value;
  if (node.type === "TemplateLiteral" && node.expressions.length === 0) return node.quasis[0].value.cooked ?? null;
  return null;
}

export function moduleConstant(doc: JsxDocument, name: string): AstNode | null {
  for (const statement of doc.ast.program.body as AstNode[]) {
    const declaration = statement.type === "ExportNamedDeclaration" ? statement.declaration : statement;
    if (!declaration || declaration.type !== "VariableDeclaration" || declaration.kind !== "const") continue;
    for (const declarator of declaration.declarations as AstNode[]) {
      if (declarator.id.type === "Identifier" && declarator.id.name === name) return unwrapExpression(declarator.init);
    }
  }
  return null;
}

export function unwrapExpression(node: AstNode | null): AstNode | null {
  let current = node;
  while (current && (current.type === "TSAsExpression" || current.type === "TSSatisfiesExpression" || current.type === "ParenthesizedExpression" || current.type === "TSNonNullExpression")) {
    current = current.expression;
  }
  return current;
}

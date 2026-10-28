import { inlineTags, trimmedRange, type MarkupElement } from "../markup/element";
import { htmlToJsx } from "../markup/html-to-jsx";
import { reactAttributeAliases, reactAttributeName } from "../markup/jsx-attrs";
import {
  enclosing,
  isFunctionNode,
  moduleConstant,
  stringLiteralValue,
  unwrapExpression,
  type AstNode,
  type JsxAttr,
  type JsxDocument,
  type JsxElement,
} from "../markup/jsx";
import { markupLanguage } from "../project";
import type { ClassList, Insertion, MarkupAdapter, Workspace } from "./adapter";
import { jsxAttributeValue, plainText, renderRuns, type Run } from "./inline";
import { Patcher } from "./patcher";
import { intoInsertion } from "./placement";
import { OpFailure, type FileChange } from "./types";

const classHelpers = new Set(["cn", "clsx", "cx", "classnames", "classNames", "twMerge", "twJoin"]);

function meaningfulChildren(node: AstNode): AstNode[] {
  return (node.children as AstNode[]).filter((child) => !(child.type === "JSXText" && child.value.trim() === ""));
}

function inlineContent(node: AstNode): boolean {
  for (const child of node.children as AstNode[]) {
    if (child.type === "JSXText") continue;
    if (child.type === "JSXExpressionContainer") {
      const expr = child.expression as AstNode;
      if (expr.type === "JSXEmptyExpression" || stringLiteralValue(expr) !== null) continue;
      return false;
    }
    if (child.type === "JSXElement") {
      const name = child.openingElement.name;
      if (name.type !== "JSXIdentifier" || !inlineTags.has(name.name) || !inlineContent(child)) return false;
      continue;
    }
    return false;
  }
  return true;
}

type LiteralKind = "js" | "jsx-attribute" | "template";

interface Literal {
  path: string;
  node: AstNode;
  kind: LiteralKind;
}

function escapeJs(text: string, quote: string): string {
  return text
    .replace(/\\/g, "\\\\")
    .replace(new RegExp(quote, "g"), `\\${quote}`)
    .replace(/\n/g, "\\n")
    .replace(/\r/g, "\\r")
    .replace(lineSeparator, "\\u2028")
    .replace(paragraphSeparator, "\\u2029");
}

const lineSeparator = new RegExp(String.fromCharCode(0x2028), "g");
const paragraphSeparator = new RegExp(String.fromCharCode(0x2029), "g");

function writeLiteral(content: string, literal: Literal, text: string): Patcher {
  const patcher = new Patcher(content);
  const node = literal.node;
  if (literal.kind === "template") {
    patcher.replace(node.start + 1, node.end - 1, text.replace(/\\/g, "\\\\").replace(/`/g, "\\`").replace(/\$\{/g, "\\${"));
    return patcher;
  }
  if (literal.kind === "jsx-attribute") {
    if (/["&\\\n\r{}]/.test(text)) patcher.replace(node.start, node.end, `{${JSON.stringify(text)}}`);
    else patcher.replace(node.start + 1, node.end - 1, text);
    return patcher;
  }
  const quote = content[node.start];
  patcher.replace(node.start + 1, node.end - 1, escapeJs(text, quote));
  return patcher;
}

function literalOf(path: string, node: AstNode | null | undefined, kind: LiteralKind = "js"): Literal | null {
  const value = unwrapExpression(node ?? null);
  if (!value) return null;
  if (value.type === "StringLiteral") return { path, node: value, kind };
  if (value.type === "TemplateLiteral" && value.expressions.length === 0) return { path, node: value, kind: "template" };
  return null;
}

function propertyKey(prop: AstNode): string | null {
  if (prop.type !== "ObjectProperty" || prop.computed) return null;
  if (prop.key.type === "Identifier") return prop.key.name;
  if (prop.key.type === "StringLiteral") return prop.key.value;
  return null;
}

function objectField(object: AstNode | null, field: string): AstNode | null {
  const value = unwrapExpression(object);
  if (!value || value.type !== "ObjectExpression") return null;
  for (const prop of value.properties as AstNode[]) if (propertyKey(prop) === field) return prop.value;
  return null;
}

function memberField(expr: AstNode): { object: string; field: string } | null {
  if (expr.type !== "MemberExpression" && expr.type !== "OptionalMemberExpression") return null;
  if (expr.object.type !== "Identifier") return null;
  if (!expr.computed && expr.property.type === "Identifier") return { object: expr.object.name, field: expr.property.name };
  if (expr.computed && expr.property.type === "StringLiteral") return { object: expr.object.name, field: expr.property.value };
  return null;
}

function destructuredKey(pattern: AstNode, local: string): string | null {
  if (pattern.type !== "ObjectPattern") return null;
  for (const prop of pattern.properties as AstNode[]) {
    if (prop.type !== "ObjectProperty") continue;
    const value = prop.value.type === "AssignmentPattern" ? prop.value.left : prop.value;
    if (value.type === "Identifier" && value.name === local) return propertyKey(prop);
  }
  return null;
}

function soleExpression(el: JsxElement): AstNode | null {
  const children = meaningfulChildren(el.node);
  if (children.length !== 1 || children[0].type !== "JSXExpressionContainer") return null;
  const expr = children[0].expression as AstNode;
  return expr.type === "JSXEmptyExpression" ? null : expr;
}

function mapCallback(doc: JsxDocument, el: JsxElement): { fn: AstNode; array: AstNode | null } | null {
  let fn = enclosing(doc, el.node, isFunctionNode);
  while (fn) {
    const call = doc.parents.get(fn);
    if (call && call.type === "CallExpression" && call.arguments[0] === fn) {
      const callee = call.callee as AstNode;
      if ((callee.type === "MemberExpression" || callee.type === "OptionalMemberExpression") && !callee.computed && callee.property.name === "map") {
        const source = unwrapExpression(callee.object);
        if (source && source.type === "ArrayExpression") return { fn, array: source };
        if (source && source.type === "Identifier") {
          const constant = moduleConstant(doc, source.name);
          return { fn, array: constant && constant.type === "ArrayExpression" ? constant : null };
        }
        return { fn, array: null };
      }
    }
    fn = enclosing(doc, fn, isFunctionNode);
  }
  return null;
}

function arrayItemLiteral(doc: JsxDocument, el: JsxElement, item: number): Literal {
  const found = mapCallback(doc, el);
  if (!found) throw new OpFailure("field_invalid", "item");
  if (!found.array) throw new OpFailure("edit_not_found", "text");
  const entry = (found.array.elements as Array<AstNode | null>)[item];
  if (!entry) throw new OpFailure("field_invalid", "item");
  const expr = soleExpression(el);
  const param = found.fn.params[0] as AstNode | undefined;
  if (!expr || !param) throw new OpFailure("edit_not_found", "text");
  let target: AstNode | null = null;
  if (expr.type === "Identifier" && param.type === "Identifier" && expr.name === param.name) target = entry;
  const member = memberField(expr);
  if (member && param.type === "Identifier" && member.object === param.name) target = objectField(entry, member.field);
  if (expr.type === "Identifier" && param.type === "ObjectPattern") {
    const key = destructuredKey(param, expr.name);
    if (key) target = objectField(entry, key);
  }
  const literal = literalOf(doc.path, target);
  if (!literal) throw new OpFailure("edit_not_found", "text");
  return literal;
}

function componentOf(doc: JsxDocument, node: AstNode): { name: string; fn: AstNode } | null {
  let fn = enclosing(doc, node, isFunctionNode);
  while (fn) {
    if (fn.type === "FunctionDeclaration" && fn.id && /^[A-Z]/.test(fn.id.name)) return { name: fn.id.name, fn };
    const parent = doc.parents.get(fn);
    if (parent && parent.type === "VariableDeclarator" && parent.id.type === "Identifier" && /^[A-Z]/.test(parent.id.name)) return { name: parent.id.name, fn };
    fn = enclosing(doc, fn, isFunctionNode);
  }
  return null;
}

function propLiteral(ws: Workspace, doc: JsxDocument, el: JsxElement, expr: AstNode): Literal {
  const component = componentOf(doc, el.node);
  if (!component) throw new OpFailure("edit_not_found", "text");
  const param = component.fn.params[0] as AstNode | undefined;
  if (!param) throw new OpFailure("edit_not_found", "text");
  let prop: string | null = null;
  if (expr.type === "Identifier") prop = destructuredKey(param, expr.name);
  const member = memberField(expr);
  if (member && param.type === "Identifier" && member.object === param.name) prop = member.field;
  if (!prop) throw new OpFailure("edit_not_found", "text");
  const found: Literal[] = [];
  for (const path of ws.paths()) {
    if (markupLanguage(ws.stack, path) !== "jsx") continue;
    let other: JsxDocument;
    try {
      other = ws.jsx(path);
    } catch {
      continue;
    }
    for (const site of other.elements) {
      if (site.tag !== component.name) continue;
      const attr = site.attrs.find((candidate) => candidate.name === prop);
      if (!attr) continue;
      const value = attr.node.value as AstNode | null;
      const literal = value?.type === "StringLiteral" ? { path, node: value, kind: "jsx-attribute" as const } : value?.type === "JSXExpressionContainer" ? literalOf(path, value.expression) : null;
      if (!literal) throw new OpFailure("edit_not_found", "text");
      found.push(literal);
    }
  }
  if (found.length === 0) throw new OpFailure("edit_not_found", "text");
  if (found.length > 1) throw new OpFailure("edit_ambiguous", "text", `${found.length} call sites pass ${prop} to ${component.name}`);
  return found[0];
}

function expressionLiteral(ws: Workspace, doc: JsxDocument, el: JsxElement, expr: AstNode): Literal {
  const direct = literalOf(doc.path, expr);
  if (direct) return direct;
  if (expr.type === "Identifier") {
    const constant = literalOf(doc.path, moduleConstant(doc, expr.name));
    if (constant) return constant;
  }
  return propLiteral(ws, doc, el, expr);
}

function patchLiteral(ws: Workspace, literal: Literal, runs: Run[]): Record<string, FileChange> {
  const text = plainText(runs);
  if (text === null) throw new OpFailure("text_markup_invalid", "text");
  const content = ws.content(literal.path)!;
  return { [literal.path]: writeLiteral(content, literal, text).result() };
}

function attrFor(el: MarkupElement, name: string): JsxAttr | undefined {
  const aliases = el.component ? [name] : reactAttributeAliases(name);
  return (el.attrs as JsxAttr[]).find((attr) => aliases.includes(attr.name));
}

function attributeInsertPoint(el: JsxElement): number {
  const opening = el.node.openingElement as AstNode;
  const attributes = opening.attributes as AstNode[];
  if (attributes.length > 0) return attributes[attributes.length - 1].end;
  return opening.typeArguments?.end ?? opening.typeParameters?.end ?? opening.name.end;
}

function precedingSpace(content: string, start: number): number {
  let s = start;
  while (s > 0 && /[ \t\r\n]/.test(content[s - 1])) s--;
  return s;
}

function classSource(el: JsxElement): { literal: Literal | null; attr: JsxAttr | null; dynamic: boolean } {
  const attr = attrFor(el, "class") ?? null;
  if (!attr) return { literal: null, attr: null, dynamic: false };
  const value = attr.node.value as AstNode | null;
  if (!value) return { literal: null, attr, dynamic: true };
  if (value.type === "StringLiteral") return { literal: { path: "", node: value, kind: "jsx-attribute" }, attr, dynamic: false };
  if (value.type !== "JSXExpressionContainer") return { literal: null, attr, dynamic: true };
  const expr = unwrapExpression(value.expression);
  const direct = literalOf("", expr);
  if (direct) return { literal: direct, attr, dynamic: false };
  if (expr && expr.type === "CallExpression" && expr.callee.type === "Identifier" && classHelpers.has(expr.callee.name)) {
    const first = literalOf("", expr.arguments[0]);
    if (first) return { literal: first, attr, dynamic: false };
  }
  return { literal: null, attr, dynamic: true };
}

export function jsxClassesAreLiteral(el: JsxElement): boolean {
  return !classSource(el).dynamic;
}

export const jsxAdapter: MarkupAdapter = {
  element(ws: Workspace, path: string, offset: number) {
    return ws.jsx(path).at(offset);
  },

  setText(ws: Workspace, path: string, el: MarkupElement, runs: Run[], item: number | undefined): Record<string, FileChange> {
    const doc = ws.jsx(path);
    const jsx = el as JsxElement;
    if (item !== undefined) return patchLiteral(ws, arrayItemLiteral(doc, jsx, item), runs);
    if (el.innerStart === null || el.innerEnd === null) throw new OpFailure("node_invalid", "node");
    const expr = soleExpression(jsx);
    if (expr && stringLiteralValue(expr) === null) return patchLiteral(ws, expressionLiteral(ws, doc, jsx, expr), runs);
    if (!inlineContent(jsx.node)) throw new OpFailure("node_invalid", "node");
    const content = ws.content(path)!;
    const [start, end] = trimmedRange(content, el.innerStart, el.innerEnd);
    return { [path]: new Patcher(content).replace(start, end, renderRuns(runs, "jsx")).result() };
  },

  setAttr(patcher: Patcher, content: string, el: MarkupElement, name: string, value: string | null) {
    const existing = attrFor(el, name);
    if (value === null) {
      if (existing) patcher.remove(precedingSpace(content, existing.start), existing.end);
      return;
    }
    const written = jsxAttributeValue(value);
    if (existing) {
      const node = existing.node.value as AstNode | null;
      if (node) patcher.replace(node.start, node.end, written);
      else patcher.replace(existing.start, existing.end, `${existing.name}=${written}`);
      return;
    }
    const attrName = el.component ? name : reactAttributeName(name);
    patcher.insertAfter(attributeInsertPoint(el as JsxElement), ` ${attrName}=${written}`);
  },

  classes(content: string, el: MarkupElement): ClassList {
    const jsx = el as JsxElement;
    const source = classSource(jsx);
    if (source.dynamic) throw new OpFailure("node_invalid", "node");
    const literal = source.literal;
    const raw = literal ? stringLiteralValue(literal.node) ?? "" : "";
    return {
      tokens: raw.split(/\s+/).filter(Boolean),
      write(patcher: Patcher, next: string[]) {
        const value = next.join(" ");
        if (!literal) {
          if (value !== "") patcher.insertAfter(attributeInsertPoint(jsx), ` className="${value}"`);
          return;
        }
        if (value === "" && literal.kind === "jsx-attribute" && source.attr) {
          patcher.remove(precedingSpace(content, source.attr.start), source.attr.end);
          return;
        }
        patcher.replace(literal.node.start + 1, literal.node.end - 1, value);
      },
    };
  },

  isImage(el: MarkupElement) {
    return el.tag === "img";
  },

  section(html: string) {
    return htmlToJsx(html);
  },

  pageInsertion(ws: Workspace, path: string): Insertion | null {
    const doc = ws.jsx(path);
    for (const statement of doc.ast.program.body as AstNode[]) {
      if (statement.type !== "ExportDefaultDeclaration") continue;
      let fn = statement.declaration as AstNode;
      if (fn.type === "Identifier") {
        fn = (doc.ast.program.body as AstNode[])
          .map((s) => (s.type === "FunctionDeclaration" && s.id?.name === fn.name ? s : null))
          .find(Boolean) ?? fn;
      }
      if (!isFunctionNode(fn)) return null;
      let root: AstNode | null = null;
      if (fn.body.type === "JSXElement") root = fn.body;
      for (const inner of fn.body.type === "BlockStatement" ? (fn.body.body as AstNode[]) : []) {
        if (inner.type === "ReturnStatement" && inner.argument) {
          const arg = unwrapExpression(inner.argument);
          if (arg && (arg.type === "JSXElement" || arg.type === "JSXFragment")) root = arg;
        }
      }
      if (!root) return null;
      const closing = root.type === "JSXElement" ? root.closingElement : root.closingFragment;
      if (!closing) return null;
      return intoInsertion(doc.content, closing.start);
    }
    return null;
  },
};

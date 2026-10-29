import { childNodes, enclosing, isFunctionNode, type AstNode, type JsxDocument } from "../markup/jsx";

const GLOBALS = new Set([
  "undefined", "NaN", "Infinity", "globalThis", "Math", "JSON", "Date", "Number", "String", "Boolean", "Array", "Object", "Intl", "RegExp", "Symbol", "Map", "Set", "Promise", "Error",
  "parseInt", "parseFloat", "isNaN", "isFinite", "encodeURIComponent", "decodeURIComponent", "encodeURI", "decodeURI", "console", "window", "document", "navigator", "location",
]);

const TYPE_ONLY = /^TS(?!AsExpression$|NonNullExpression$|SatisfiesExpression$)/;

function patternNames(pattern: AstNode | null | undefined, out: Set<string>, defaults: AstNode[]) {
  if (!pattern) return;
  switch (pattern.type) {
    case "Identifier":
      out.add(pattern.name);
      return;
    case "ObjectPattern":
      for (const property of pattern.properties as AstNode[]) patternNames(property.type === "RestElement" ? property.argument : property.value, out, defaults);
      return;
    case "ArrayPattern":
      for (const element of pattern.elements as Array<AstNode | null>) patternNames(element, out, defaults);
      return;
    case "AssignmentPattern":
      defaults.push(pattern.right);
      patternNames(pattern.left, out, defaults);
      return;
    case "RestElement":
      patternNames(pattern.argument, out, defaults);
  }
}

function declaredIn(statements: readonly AstNode[], out: Set<string>) {
  for (const raw of statements) {
    const statement = (raw.type === "ExportNamedDeclaration" || raw.type === "ExportDefaultDeclaration") && raw.declaration ? raw.declaration : raw;
    if (statement.type === "VariableDeclaration") for (const declarator of statement.declarations as AstNode[]) patternNames(declarator.id, out, []);
    if ((statement.type === "FunctionDeclaration" || statement.type === "ClassDeclaration") && statement.id) out.add(statement.id.name);
    if (statement.type === "ImportDeclaration") for (const specifier of statement.specifiers as AstNode[]) out.add(specifier.local.name);
  }
}

function functionNames(fn: AstNode, out: Set<string>) {
  for (const param of fn.params as AstNode[]) patternNames(param, out, []);
  if (fn.body?.type === "BlockStatement") declaredIn(fn.body.body as AstNode[], out);
}

export function freeNames(root: AstNode): string[] {
  const found = new Set<string>();
  const visit = (node: AstNode, bound: ReadonlySet<string>) => {
    if (TYPE_ONLY.test(node.type)) return;
    if (node.type === "Identifier") {
      if (!bound.has(node.name)) found.add(node.name);
      return;
    }
    if (node.type === "JSXOpeningElement") {
      let name = node.name as AstNode;
      while (name.type === "JSXMemberExpression") name = name.object;
      if (name.type === "JSXIdentifier" && /^[A-Z]/.test(name.name) && !bound.has(name.name)) found.add(name.name);
      for (const attribute of node.attributes as AstNode[]) visit(attribute, bound);
      return;
    }
    if (node.type === "JSXClosingElement" || node.type === "JSXIdentifier" || node.type === "JSXNamespacedName") return;
    if (node.type === "MemberExpression" || node.type === "OptionalMemberExpression") {
      visit(node.object, bound);
      if (node.computed) visit(node.property, bound);
      return;
    }
    if (node.type === "ObjectProperty") {
      if (node.computed) visit(node.key, bound);
      visit(node.value, bound);
      return;
    }
    if (node.type === "ArrowFunctionExpression" || node.type === "FunctionExpression") {
      const inner = new Set(bound);
      const defaults: AstNode[] = [];
      for (const param of node.params as AstNode[]) patternNames(param, inner, defaults);
      if (node.id) inner.add(node.id.name);
      if (node.body.type === "BlockStatement") declaredIn(node.body.body as AstNode[], inner);
      defaults.forEach((value) => visit(value, bound));
      visit(node.body, inner);
      return;
    }
    if (node.type === "VariableDeclarator") {
      if (node.init) visit(node.init, bound);
      return;
    }
    for (const child of childNodes(node)) visit(child, bound);
  };
  visit(root, new Set());
  return [...found].filter((name) => !GLOBALS.has(name)).sort();
}

export function scopeNames(doc: JsxDocument, landing: AstNode | null): Set<string> {
  const out = new Set<string>();
  const program = doc.ast.program.body as AstNode[];
  declaredIn(program, out);
  if (landing) {
    if (isFunctionNode(landing)) functionNames(landing, out);
    for (let fn = enclosing(doc, landing, isFunctionNode); fn; fn = enclosing(doc, fn, isFunctionNode)) functionNames(fn, out);
  }
  return out;
}

export function pageFunction(doc: JsxDocument): AstNode | null {
  const program = doc.ast.program.body as AstNode[];
  const exported = program.find((statement) => statement.type === "ExportDefaultDeclaration")?.declaration as AstNode | undefined;
  if (!exported) return null;
  if (exported.type !== "Identifier") return isFunctionNode(exported) ? exported : null;
  return program.find((statement) => statement.type === "FunctionDeclaration" && statement.id?.name === exported.name) ?? null;
}

export function missingReferences(copied: AstNode, target: JsxDocument, landing: AstNode | null): string[] {
  const available = scopeNames(target, landing);
  return freeNames(copied).filter((name) => !available.has(name));
}

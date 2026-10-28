import { jsxClassesAreLiteral } from "../execute/jsx-adapter";
import { enclosing, isFunctionNode, parseJsx, parseModule, walk, type AstNode, type JsxDocument } from "../markup/jsx";

export const importForbidden = "import_forbidden";
export const importDynamicForbidden = "import_dynamic_forbidden";

export interface SourceRefusal {
  line: number;
  column: number;
  code: string;
  message: string;
}

export interface SourcePolicyResult {
  specifiers: string[];
  refusals: SourceRefusal[];
}

const workerConstructors = new Set(["Worker", "SharedWorker"]);

const keyParents = new Set(["ObjectProperty", "ObjectMethod", "ClassProperty", "ClassMethod", "ClassPrivateProperty", "ClassAccessorProperty", "TSPropertySignature", "TSMethodSignature", "TSDeclareMethod", "TSAbstractMethodDefinition", "TSEnumMember"]);

function propertyName(node: AstNode): string | null {
  if (node.type === "Identifier") return node.name;
  if (node.type === "StringLiteral") return node.value;
  return null;
}

function nameOnly(node: AstNode, parent: AstNode | null): boolean {
  if (!parent) return false;
  if ((parent.type === "MemberExpression" || parent.type === "OptionalMemberExpression") && parent.property === node && !parent.computed) return true;
  if (keyParents.has(parent.type) && parent.key === node && !parent.computed) return true;
  if (parent.type === "TSQualifiedName" && parent.right === node) return true;
  if ((parent.type === "LabeledStatement" || parent.type === "BreakStatement" || parent.type === "ContinueStatement") && parent.label === node) return true;
  return false;
}

function literalRequire(node: AstNode): boolean {
  return node.arguments.length === 1 && node.arguments[0].type === "StringLiteral";
}

function hasAttributes(node: AstNode): boolean {
  return (Array.isArray(node.attributes) && node.attributes.length > 0) || (Array.isArray(node.assertions) && node.assertions.length > 0) || node.extra?.deprecatedAssertSyntax === true;
}

function serviceWorkerRegister(callee: AstNode): boolean {
  if (callee.type !== "MemberExpression" && callee.type !== "OptionalMemberExpression") return false;
  if (propertyName(callee.property) !== "register") return false;
  const owner = callee.object;
  return (owner.type === "MemberExpression" || owner.type === "OptionalMemberExpression") && propertyName(owner.property) === "serviceWorker";
}

export function sourcePolicy(content: string, path: string, refuse: (specifier: string) => string | null): SourcePolicyResult {
  let ast: AstNode;
  try {
    ast = parseModule(content, path);
  } catch (error) {
    const loc = (error as { loc?: { line: number; column: number } }).loc;
    const message = (error as Error).message.replace(/\s*\(\d+:\d+\)$/, "");
    return { specifiers: [], refusals: [{ line: loc?.line ?? 0, column: (loc?.column ?? 0) + 1, code: "", message }] };
  }
  const specifiers: string[] = [];
  const refusals: SourceRefusal[] = [];
  const at = (node: AstNode) => ({ line: node.loc?.start.line ?? 0, column: (node.loc?.start.column ?? 0) + 1 });
  const dynamic = (node: AstNode, message: string) => refusals.push({ ...at(node), code: importDynamicForbidden, message });
  const specifier = (node: AstNode, value: string) => {
    specifiers.push(value);
    const refused = refuse(value);
    if (refused !== null) refusals.push({ ...at(node), code: importForbidden, message: refused });
  };
  const requireCallees = new Set<AstNode>();
  walk(ast, (node, parent) => {
    switch (node.type) {
      case "ImportDeclaration":
      case "ExportAllDeclaration":
      case "ExportNamedDeclaration":
        if (!node.source) return;
        if (hasAttributes(node)) dynamic(node, "import attributes are not allowed");
        specifier(node, node.source.value);
        return;
      case "TSImportEqualsDeclaration":
        if (node.moduleReference?.type === "TSExternalModuleReference") specifier(node, node.moduleReference.expression.value);
        return;
      case "ImportExpression":
        if (node.options) dynamic(node, "import attributes are not allowed");
        if (node.source.type !== "StringLiteral") dynamic(node, "a dynamic import must name a fixed file or library");
        else specifier(node, node.source.value);
        return;
      case "CallExpression":
      case "OptionalCallExpression": {
        const callee = node.callee as AstNode;
        if (callee.type === "Import") {
          if (node.arguments.length !== 1 || node.arguments[0].type !== "StringLiteral") dynamic(node, "a dynamic import must name a fixed file or library");
          else specifier(node, node.arguments[0].value);
          return;
        }
        if (callee.type === "Identifier" && callee.name === "require") {
          requireCallees.add(callee);
          if (!literalRequire(node)) dynamic(node, "require must name a fixed file or library");
          else specifier(node, node.arguments[0].value);
          return;
        }
        if (serviceWorkerRegister(callee)) dynamic(node, "service workers are not allowed");
        return;
      }
      case "NewExpression":
        if (node.callee.type === "Identifier" && workerConstructors.has(node.callee.name)) dynamic(node, "web workers are not allowed");
        return;
      case "MetaProperty":
        if (node.meta.name === "import") dynamic(node, "import.meta is not allowed");
        return;
      case "Identifier":
        if (node.name === "require" && !requireCallees.has(node) && !nameOnly(node, parent)) dynamic(node, "require can only be called with a fixed file or library");
        return;
    }
  });
  return { specifiers, refusals };
}

export interface BuildWarning {
  path: string;
  line: number;
  column: number;
  code: string;
  message: string;
}

const textTags = new Set(["h1", "h2", "h3", "h4", "h5", "h6", "p", "span", "a", "button", "li", "blockquote", "label", "strong", "em"]);

function position(node: AstNode): { line: number; column: number } {
  return { line: node.loc?.start.line ?? 1, column: (node.loc?.start.column ?? 0) + 1 };
}

function hasText(node: AstNode): boolean {
  return (node.children as AstNode[]).some((child) => (child.type === "JSXText" && child.value.trim() !== "") || child.type === "JSXExpressionContainer");
}

function hidesWithOpacity(attr: AstNode | undefined): boolean {
  const value = attr?.value as AstNode | null | undefined;
  if (!value || value.type !== "JSXExpressionContainer") return false;
  const expr = value.expression as AstNode;
  if (expr.type !== "ObjectExpression") return false;
  return (expr.properties as AstNode[]).some((prop) => {
    if (prop.type !== "ObjectProperty") return false;
    const key = prop.key.type === "Identifier" ? prop.key.name : prop.key.value;
    return key === "opacity" && prop.value.type === "NumericLiteral" && prop.value.value === 0;
  });
}

function componentFunction(doc: JsxDocument, node: AstNode): AstNode | null {
  const fn = enclosing(doc, node, isFunctionNode);
  if (!fn) return null;
  if (fn.type === "FunctionDeclaration" && fn.id && /^[A-Z]/.test(fn.id.name)) return fn;
  const parent = doc.parents.get(fn);
  if (parent?.type === "VariableDeclarator" && parent.id.type === "Identifier" && /^[A-Z]/.test(parent.id.name)) return fn;
  if (parent?.type === "ExportDefaultDeclaration") return fn;
  return null;
}

function nondeterministic(node: AstNode): string | null {
  if (node.type === "CallExpression" && node.callee.type === "MemberExpression" && !node.callee.computed) {
    const object = node.callee.object;
    const property = node.callee.property.name;
    if (object.type === "Identifier" && object.name === "Math" && property === "random") return "Math.random()";
    if (object.type === "Identifier" && object.name === "Date" && property === "now") return "Date.now()";
  }
  if (node.type === "NewExpression" && node.callee.type === "Identifier" && node.callee.name === "Date" && node.arguments.length === 0) return "new Date()";
  return null;
}

export function lintModule(content: string, path: string): BuildWarning[] {
  let doc: JsxDocument;
  try {
    doc = parseJsx(content, path);
  } catch {
    return [];
  }
  const warnings: BuildWarning[] = [];
  const warn = (node: AstNode, code: string, message: string) => warnings.push({ path, ...position(node), code, message });
  for (const el of doc.elements) {
    if (el.component && !el.tag.includes(".")) continue;
    if (!jsxClassesAreLiteral(el)) warn(el.node, "class_dynamic", `className of <${el.tag}> is not a literal, so the inspector cannot edit it`);
    const src = el.attrs.find((attr) => attr.name === "src");
    if (el.tag === "img" && src && !src.literal) warn(src.node, "image_src_dynamic", "the image src is not a literal, so Trocar imagem cannot replace it");
    if (el.tag.startsWith("motion.")) {
      const initial = el.attrs.find((attr) => attr.name === "initial");
      const tag = el.tag.slice("motion.".length);
      if (hidesWithOpacity(initial?.node) && (textTags.has(tag) || hasText(el.node))) {
        warn(el.node, "motion_hidden_text", `<${el.tag}> starts at opacity 0, so its text is invisible in the prerendered page`);
      }
    }
  }
  walk(doc.ast, (node) => {
    const what = nondeterministic(node);
    if (!what) return;
    const fn = componentFunction(doc, node);
    if (fn) warn(node, "render_nondeterministic", `${what} during render makes the prerendered page differ from hydration`);
  });
  return warnings;
}

const hookCall = /^use[A-Z0-9]/;

export function interactiveModule(content: string, path: string): boolean {
  let doc: JsxDocument;
  try {
    doc = parseJsx(content, path);
  } catch {
    return true;
  }
  let interactive = false;
  walk(doc.ast, (node) => {
    if (interactive) return;
    if (node.type === "CallExpression") {
      const callee = node.callee as AstNode;
      if (callee.type === "Identifier" && hookCall.test(callee.name)) interactive = true;
      if (callee.type === "MemberExpression" && !callee.computed && hookCall.test(callee.property.name ?? "")) interactive = true;
    }
    if (node.type === "JSXAttribute" && node.name.type === "JSXIdentifier" && /^on[A-Z]/.test(node.name.name)) interactive = true;
    if ((node.type === "ClassDeclaration" || node.type === "ClassExpression") && node.superClass) interactive = true;
  });
  return interactive;
}

export const staticPackages = new Set(["react", "react/jsx-runtime", "react-dom", "react-dom/client", "clsx", "tailwind-merge", "class-variance-authority", "lucide-react", "lucide", "@radix-ui/react-slot"]);

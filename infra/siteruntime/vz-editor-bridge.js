const EDITOR_ORIGIN_PARAM = "vz-editor";
const ORIGIN_STORAGE_KEY = "vz-editor-origin";
const MAX_TEXT = 20000;
const MAX_ITEMS = 500;
const NODE = /^(?!.*\.\.)[A-Za-z0-9_][A-Za-z0-9_./-]{0,255}#(0|[1-9][0-9]{0,8})$/;
const TAG = /^[a-z][a-z0-9-]{0,63}$/;
const PATH_SEGMENT = /^[a-z][a-z0-9-]{0,63}:(0|[1-9][0-9]{0,5})$/;
const ATTR_NAME = /^[^\s"'<>/=]{1,128}$/;
const CLASS_NAME = /^\S{1,256}$/;
const REL = /^[a-z]+( [a-z]+)*$/;
const LOCALES = ["pt", "en", "es", "de"];
const TOOLBAR_LABELS = {
  pt: ["Negrito", "Itálico", "Link", "Endereço do link"],
  en: ["Bold", "Italic", "Link", "Link address"],
  es: ["Negrita", "Cursiva", "Enlace", "Dirección del enlace"],
  de: ["Fett", "Kursiv", "Link", "Linkadresse"],
};
const KEPT_TAGS = new Set(["B", "STRONG", "I", "EM"]);
const DROPPED_TAGS = new Set(["SCRIPT", "STYLE", "TEMPLATE", "NOSCRIPT"]);
const INLINE_TAGS = new Set(["B", "STRONG", "I", "EM", "A", "BR", "SPAN", "SMALL", "MARK", "CODE", "SUP", "SUB", "U", "S"]);
const CAPTURE_ID = /^[A-Za-z0-9_-]{1,64}$/;
const JPEG_URL = /^data:image\/jpeg;base64,[A-Za-z0-9+/=]*$/;
const MAX_DOCUMENT = 1572864;
const MAX_IMAGE = 16000000;
const MAX_CAPTURE_HEIGHT = 4000;
const MAX_BOXES = 400;
const MAX_TEXTS = 300;
const MAX_IMAGES = 150;
const MAX_OVERFLOW = 3;
const MAX_ERRORS = 20;
const MAX_ERROR_TEXT = 500;
const MAX_URL = 500;
const MAX_CAPTURE_PIXELS = 6000000;
const MAX_VIEWPORTS_DOWN = 24;
const MIN_BOX_WIDTH = 16;
const MIN_BOX_HEIGHT = 12;
const MAX_SCAN = 5000;
const MAX_OFFENDERS = 300;
const SCROLL_SETTLE_MS = 120;
const FONT_WAIT_MS = 3000;
const JPEG_QUALITY = 0.85;
const OVERLAY_TAG = "vz-bridge-overlay";
const RAW_TEXT = new Set(["script", "style", "xmp", "noembed", "noframes", "plaintext"]);
const VOID_TAGS = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"]);
const MEDIA_TAGS = new Set(["IMG", "VIDEO", "CANVAS", "PICTURE", "IFRAME", "svg"]);
const SKIP_TEXT = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "TEMPLATE", "OPTION", "TITLE"]);
const CAPTURE_FAILURES = ["focus_missing", "too_large", "capture_failed"];
const STYLE_NAME = /^[a-z-]{1,40}$/;
const MAX_STYLES = 64;
const MAX_STYLE_TEXT = 200;
const MAX_CHILD_BOXES = 48;
const PAINT_WAIT_MS = 250;
const COMPUTED_STYLES = [
  "display", "flex-direction", "flex-wrap", "justify-content", "align-items", "grid-template-columns", "row-gap", "column-gap",
  "padding-top", "padding-right", "padding-bottom", "padding-left", "margin-top", "margin-right", "margin-bottom", "margin-left",
  "width", "max-width", "height", "min-height", "aspect-ratio", "object-fit",
  "font-family", "font-size", "font-weight", "font-style", "line-height", "letter-spacing", "text-align", "text-transform", "text-decoration-line", "color",
  "background-color", "border-top-width", "border-top-style", "border-top-color", "border-top-left-radius",
  "box-shadow", "opacity", "filter", "rotate", "scale", "translate", "transform", "transform-origin",
];
export const PLATFORM_TAGS = Object.freeze(["vz-webchat", "vz-booking"]);
export const INHERITED_STYLES = Object.freeze(["color", "font-family", "font-size", "font-weight", "font-style", "line-height", "letter-spacing", "text-align", "text-transform"]);
const PLATFORM_SELECTOR = PLATFORM_TAGS.join(", ");
const INSTANCE_ATTR = "data-vz-inst";
const DESIGN_ATTR = "data-vz-design";
const HOVER_GUARD = ":not(html[" + DESIGN_ATTR + "] *)";
const UNGUARDED_HOVER = /:hover(?!:not\(html\[data-vz-design\] \*\))/g;
const HOLDS_NOTHING = new Set([
  "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr",
  "p", "h1", "h2", "h3", "h4", "h5", "h6", "span", "a", "button", "label", "strong", "em", "b", "i", "small", "u", "s", "mark", "sup", "sub", "code", "pre",
  "option", "select", "textarea", "svg", "video", "audio", "iframe", "picture", "canvas", "script", "style", "template", "noscript", "vz-bridge-overlay",
]);
const LONG_PRESS_MS = 450;
const PRESS_SLOP = 6;
const EDGE_BAND = 24;
const SCROLL_BAND = 40;
const SCROLL_STEP = 14;
const FORBIDDEN_LIVE_ATTRS = new Set(["style", "class", "classname", "srcdoc"]);
const URL_ATTRS = new Set(["href", "src", "action", "formaction", "poster", "xlink:href", "data", "cite", "background"]);
const CREDENTIAL_TOKENS = new Set(["current-password", "new-password", "one-time-code"]);
const LIVE_ATTR_NAME = /^[a-z_:][a-z0-9_.:-]*$/;
const LIVE_CLASS = /^[A-Za-z0-9!_:./[\]#%(),+*=&@-]{1,200}$/;
const FORBIDDEN_LIVE_TAGS = "script, iframe, object, embed, frame, frameset";
const APPLY_REFUSALS = ["node_unknown", "attribute_forbidden", "class_invalid", "element_forbidden"];

function isPlain(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function optional(check) {
  return Object.assign((value) => check(value), { optional: true });
}

function exact(shape) {
  const keys = Object.keys(shape);
  return (value) =>
    isPlain(value) &&
    Object.keys(value).every((key) => Object.hasOwn(shape, key)) &&
    keys.every((key) => (Object.hasOwn(value, key) ? shape[key](value[key]) : shape[key].optional === true));
}

const oneOf = (...options) => (value) => typeof value === "string" && options.includes(value);
const listUpTo = (limit, item) => (value) => Array.isArray(value) && value.length <= limit && value.every(item);
const listOf = (item) => listUpTo(MAX_ITEMS, item);
const matching = (pattern) => (value) => typeof value === "string" && pattern.test(value);
const text = (value) => typeof value === "string" && value.length <= MAX_TEXT;
const finite = (value) => typeof value === "number" && Number.isFinite(value);
const size = (value) => finite(value) && value >= 0;
const unit = (value) => finite(value) && value >= 0 && value <= 1;
const line = (value) => Number.isInteger(value) && value >= 0;
const node = matching(NODE);
const optionalCount = (value) => value === null || line(value);
const rect = exact({ x: finite, y: finite, width: size, height: size });
const point = exact({ x: unit, y: unit });
const attrs = (value) => isPlain(value) && Object.keys(value).length <= MAX_ITEMS && Object.entries(value).every(([key, item]) => ATTR_NAME.test(key) && text(item));
const integer = (value) => Number.isInteger(value);
const textUpTo = (limit) => (value) => typeof value === "string" && value.length <= limit;
const captureId = matching(CAPTURE_ID);
const nodeOrEmpty = (value) => value === "" || node(value);
const channel = (value) => Number.isInteger(value) && value >= 0 && value <= 255;
const rgb = (value) => Array.isArray(value) && value.length === 3 && value.every(channel);
const captureHeight = (value) => Number.isInteger(value) && value >= 1 && value <= MAX_CAPTURE_HEIGHT;
const boxList = listUpTo(MAX_CHILD_BOXES, rect);
const styles = (value) => isPlain(value) && Object.keys(value).length <= MAX_STYLES && Object.entries(value).every(([key, item]) => STYLE_NAME.test(key) && typeof item === "string" && item.length <= MAX_STYLE_TEXT);
const classList = listOf(matching(CLASS_NAME));
const family = exact({ parent: nodeOrEmpty, previous: nodeOrEmpty, next: nodeOrEmpty });
const styleSources = (value) => isPlain(value) && Object.keys(value).length <= INHERITED_STYLES.length && Object.entries(value).every(([key, item]) => INHERITED_STYLES.includes(key) && exact({ node, tag: matching(TAG) })(item));
const LIVE_CHANGES = {
  classes: exact({ kind: () => true, add: classList, remove: classList }),
  attr: exact({ kind: () => true, name: matching(ATTR_NAME), value: (value) => value === null || text(value) }),
  remove: exact({ kind: () => true }),
  duplicate: exact({ kind: () => true }),
  move: exact({ kind: () => true, anchor: node, place: oneOf("before", "after", "into") }),
};
const dropSlot = exact({ container: nodeOrEmpty, tag: matching(TAG), anchor: nodeOrEmpty, place: oneOf("before", "after", "into"), axis: oneOf("x", "y"), refused: optional(oneOf("inside_self")) });
const liveChange = (value) => isPlain(value) && typeof value.kind === "string" && Object.hasOwn(LIVE_CHANGES, value.kind) && LIVE_CHANGES[value.kind](value);
const jpeg = (value) => typeof value === "string" && value.length <= MAX_IMAGE && JPEG_URL.test(value);
const scaleOf = (value) => finite(value) && value > 0 && value <= 1;
const status = (value) => Number.isInteger(value) && value >= 400 && value <= 599;
const box = exact({ node, tag: matching(TAG), x: integer, y: integer, w: line, h: line });
const span = exact({ top: integer, bottom: integer });
const facts = exact({
  scrollWidth: line,
  overflow: listUpTo(MAX_OVERFLOW, node),
  texts: listUpTo(MAX_TEXTS, exact({ node: nodeOrEmpty, fg: rgb, bg: rgb, px: size, weight: line })),
  images: listUpTo(MAX_IMAGES, exact({ node: nodeOrEmpty, naturalW: line, naturalH: line, shownW: line, shownH: line })),
  metrics: exact({ lcp: size, cls: size, tbt: size }),
  consoleErrors: listUpTo(MAX_ERRORS, textUpTo(MAX_ERROR_TEXT)),
  failedRequests: listUpTo(MAX_ERRORS, exact({ url: textUpTo(MAX_URL), status })),
  transferBytes: line,
});
const capturedDocument = exact({ type: () => true, id: captureId, kind: oneOf("document"), html: textUpTo(MAX_DOCUMENT) });
const capturedImage = exact({
  type: () => true,
  id: captureId,
  kind: oneOf("image"),
  image: jpeg,
  width: line,
  pageHeight: line,
  capturedHeight: line,
  scale: scaleOf,
  truncated: (value) => typeof value === "boolean",
  boxes: listUpTo(MAX_BOXES, box),
  focus: (value) => value === null || span(value),
  facts,
});

const FRAME_SHAPES = {
  "vz:ready": {},
  "vz:save": {},
  "vz:hover": { node, rect, rev: optionalCount },
  "vz:select": { node, rect, tag: matching(TAG), text, attrs, classes: listOf(matching(CLASS_NAME)), path: listOf(matching(PATH_SEGMENT)), rev: optionalCount, item: optionalCount, computed: optional(styles), rem: optional(size), parent: optional(rect), children: optional(boxList), inherited: optional(styleSources), family: optional(family), kind: optional(oneOf("icon")) },
  "vz:styles": { node, classes: classList, computed: styles, rem: size, rect: optional(rect), parent: optional(rect), children: optional(boxList), inherited: optional(styleSources) },
  "vz:apply-refused": { id: captureId, reason: oneOf(...APPLY_REFUSALS) },
  "vz:applied": { id: captureId },
  "vz:text-begin": { node },
  "vz:text-cancel": { node },
  "vz:text-commit": { node, html: text, text, rev: optionalCount, item: optionalCount },
  "vz:scroll": { y: finite },
  "vz:console": { level: oneOf("log", "info", "warn", "error", "debug"), message: text },
  "vz:error": { message: text, source: text, line },
  "vz:comment-pin": { node, rect, point },
  "vz:context": { node: nodeOrEmpty, x: finite, y: finite },
  "vz:captured": (value) => capturedDocument(value) || capturedImage(value),
  "vz:capture-failed": { id: captureId, reason: oneOf(...CAPTURE_FAILURES) },
  "vz:drop": { slot: (value) => value === null || dropSlot(value), x: optional(finite), y: optional(finite) },
  "vz:drag-begin": { node, item: optionalCount },
  "vz:drag-end": { commit: (value) => typeof value === "boolean" },
  "vz:shortcut": { action: oneOf("copy", "paste", "duplicate") },
};

const EDITOR_SHAPES = {
  "vz:highlight": { nodes: listOf(exact({ node, kind: oneOf("selection", "hover", "elo"), item: optional(optionalCount) })) },
  "vz:select-node": { node, item: optional(optionalCount), path: optional(listOf(matching(PATH_SEGMENT))) },
  "vz:begin-text-edit": { node },
  "vz:scroll-to": { node },
  "vz:scroll-y": { y: finite, anchor: optional(exact({ node, top: finite, item: optional(optionalCount), path: optional(listOf(matching(PATH_SEGMENT))) })) },
  "vz:mode": { mode: oneOf("select", "comment", "preview"), locale: optional(oneOf(...LOCALES)) },
  "vz:capture": { id: captureId, kind: oneOf("document", "image"), focus: optional(node), height: optional(captureHeight) },
  "vz:apply": { id: captureId, node, item: optional(optionalCount), change: liveChange },
  "vz:revert": { id: captureId },
  "vz:probe": { x: finite, y: finite, moving: nodeOrEmpty, item: optional(optionalCount) },
  "vz:probe-step": { direction: oneOf("previous", "next", "out", "in"), moving: nodeOrEmpty, item: optional(optionalCount) },
  "vz:probe-end": {},
  "vz:hover-states": { on: (value) => typeof value === "boolean" },
};

function decode(shapes, data) {
  if (!isPlain(data) || typeof data.type !== "string" || !Object.hasOwn(shapes, data.type)) return null;
  const shape = shapes[data.type];
  const check = typeof shape === "function" ? shape : exact({ type: () => true, ...shape });
  return check(data) ? data : null;
}

export function decodeFrameMessage(data) {
  return decode(FRAME_SHAPES, data);
}

export function decodeEditorMessage(data) {
  return decode(EDITOR_SHAPES, data);
}

function isOrigin(value) {
  try {
    const url = new URL(value);
    return (url.protocol === "https:" || url.protocol === "http:") && url.origin === value;
  } catch {
    return false;
  }
}

export function editorOriginFrom(hash, ancestorOrigins) {
  const origin = new URLSearchParams(String(hash || "").replace(/^#/, "")).get(EDITOR_ORIGIN_PARAM);
  if (!origin || !isOrigin(origin)) return null;
  if (ancestorOrigins && ancestorOrigins[0] !== origin) return null;
  return origin;
}

export function acceptEditorMessage(event, editorOrigin, parent) {
  if (!editorOrigin || event.origin !== editorOrigin || event.source !== parent) return null;
  return decodeEditorMessage(event.data);
}

function escapeText(value) {
  return value.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeAttr(value) {
  return escapeText(value).replace(/"/g, "&quot;");
}

function safeHref(raw) {
  const href = String(raw || "").replace(/[\u0000-\u001f\u007f]/g, "").trim();
  if (href === "" || href.startsWith("//")) return null;
  if (/^(https?:|mailto:|tel:)/i.test(href)) return href;
  return /^[a-z][a-z0-9+.-]*:/i.test(href) ? null : href;
}

function linkAttrs(node) {
  const href = safeHref(node.getAttribute("href"));
  if (href === null) return null;
  const blank = node.getAttribute("target") === "_blank";
  const given = (node.getAttribute("rel") || "").trim();
  let rel = REL.test(given) ? given : "";
  if (blank && !rel.split(" ").includes("noopener")) rel = (rel + " noopener").trim();
  return ' href="' + escapeAttr(href) + '"' + (blank ? ' target="_blank"' : "") + (rel ? ' rel="' + escapeAttr(rel) + '"' : "");
}

export function sanitizeInline(element) {
  let html = "";
  let plain = "";
  const walk = (parent) => {
    for (const child of parent.childNodes) {
      if (child.nodeType === 3) {
        html += escapeText(child.nodeValue);
        plain += child.nodeValue;
        continue;
      }
      if (child.nodeType !== 1 || DROPPED_TAGS.has(child.nodeName)) continue;
      if (child.nodeName === "BR") {
        html += " ";
        plain += " ";
        continue;
      }
      const tag = child.nodeName.toLowerCase();
      const opening = KEPT_TAGS.has(child.nodeName) ? "" : child.nodeName === "A" ? linkAttrs(child) : null;
      if (opening === null) {
        walk(child);
        continue;
      }
      html += "<" + tag + opening + ">";
      walk(child);
      html += "</" + tag + ">";
    }
  };
  walk(element);
  return { html, text: plain };
}

export function isInlineOnly(element) {
  for (const child of element.children) {
    if (!INLINE_TAGS.has(child.tagName) || !isInlineOnly(child)) return false;
  }
  return true;
}

export function elementPath(element) {
  const path = [];
  for (let current = element; current.parentElement && current.tagName !== "BODY"; current = current.parentElement) {
    path.unshift(current.tagName.toLowerCase() + ":" + Array.prototype.indexOf.call(current.parentElement.children, current));
  }
  return path.length > MAX_ITEMS ? path.slice(-MAX_ITEMS) : path;
}

const clamp = (value) => Math.min(1, Math.max(0, value));

const FILE = /^(?!.*\.\.)[A-Za-z0-9_][A-Za-z0-9_./-]{0,255}$/;
const INDEX = /^(0|[1-9][0-9]{0,8})$/;

export function revisionsFrom(content) {
  let parsed = null;
  try {
    parsed = JSON.parse(content);
  } catch {
    return {};
  }
  const revs = {};
  if (!isPlain(parsed)) return revs;
  for (const [file, rev] of Object.entries(parsed)) {
    if (FILE.test(file) && line(rev)) revs[file] = rev;
  }
  return revs;
}

export function revisionOf(revs, nodeRef) {
  const file = nodeRef.slice(0, nodeRef.lastIndexOf("#"));
  return Object.hasOwn(revs, file) ? revs[file] : null;
}

export function itemIndex(value) {
  return typeof value === "string" && INDEX.test(value) ? Number(value) : null;
}

export function pinPoint(box, clientX, clientY) {
  return {
    x: box.width > 0 ? clamp((clientX - box.left) / box.width) : 0,
    y: box.height > 0 ? clamp((clientY - box.top) / box.height) : 0,
  };
}

function itemOf(element) {
  const owner = element.closest("[data-vz-i]");
  return owner ? itemIndex(owner.getAttribute("data-vz-i")) : null;
}

function rectOf(element) {
  const box = element.getBoundingClientRect();
  return { x: box.left, y: box.top, width: Math.max(0, box.width), height: Math.max(0, box.height) };
}

function waitFrames(win, count) {
  return new Promise((resolve) => {
    const step = (left) => (left <= 0 ? resolve() : win.requestAnimationFrame(() => step(left - 1)));
    step(count);
  });
}

function delay(win, ms) {
  return new Promise((resolve) => win.setTimeout(resolve, ms));
}

export async function whenPainted(win) {
  const doc = win.document;
  if (doc.fonts && doc.fonts.ready) await Promise.race([doc.fonts.ready.catch(() => null), delay(win, FONT_WAIT_MS)]);
  await Promise.race([waitFrames(win, 2), delay(win, PAINT_WAIT_MS)]);
}

function contentBox(element) {
  const box = element.getBoundingClientRect();
  const style = element.ownerDocument.defaultView.getComputedStyle(element);
  const read = (name) => {
    const value = Number.parseFloat(style.getPropertyValue(name));
    return Number.isFinite(value) ? value : 0;
  };
  const left = read("border-left-width") + read("padding-left");
  const top = read("border-top-width") + read("padding-top");
  const right = read("border-right-width") + read("padding-right");
  const bottom = read("border-bottom-width") + read("padding-bottom");
  return { x: box.left + left, y: box.top + top, width: Math.max(0, box.width - left - right), height: Math.max(0, box.height - top - bottom) };
}

function shownChildren(element) {
  const view = element.ownerDocument.defaultView;
  return Array.from(element.children)
    .filter((child) => child.tagName !== OVERLAY_TAG.toUpperCase() && view.getComputedStyle(child).display !== "none")
    .slice(0, MAX_CHILD_BOXES)
    .map(rectOf);
}

function geometryOf(element) {
  const parent = element.parentElement;
  return { ...(parent ? { parent: contentBox(parent) } : {}), children: shownChildren(element) };
}

function clip(value) {
  return String(value).slice(0, MAX_TEXT);
}

function computedOf(element) {
  const view = element.ownerDocument.defaultView;
  const style = view.getComputedStyle(element);
  const out = {};
  for (const name of COMPUTED_STYLES) {
    const value = style.getPropertyValue(name);
    if (value) out[name] = String(value).slice(0, MAX_STYLE_TEXT);
  }
  return out;
}

function rootSize(element) {
  const view = element.ownerDocument.defaultView;
  const size = Number.parseFloat(view.getComputedStyle(element.ownerDocument.documentElement).fontSize);
  return Number.isFinite(size) && size > 0 ? size : 16;
}

export function platformElement(element) {
  const root = element && element.closest ? element.closest(PLATFORM_SELECTOR) : null;
  return root && NODE.test(root.getAttribute("data-vz-node") || "") ? root : null;
}

export function guardHover(selector) {
  return selector.replace(UNGUARDED_HOVER, ":hover" + HOVER_GUARD);
}

function guardRules(rules) {
  for (const rule of Array.from(rules || [])) {
    if (typeof rule.selectorText === "string" && rule.selectorText.includes(":hover")) {
      const guarded = guardHover(rule.selectorText);
      if (guarded !== rule.selectorText) rule.selectorText = guarded;
    }
    let inner = null;
    try {
      inner = rule.cssRules;
    } catch {
      inner = null;
    }
    if (inner && inner.length) guardRules(inner);
  }
}

export function guardHoverSheets(doc) {
  for (const sheet of Array.from(doc.styleSheets || [])) {
    try {
      guardRules(sheet.cssRules);
    } catch {
      continue;
    }
  }
}

export function iconInstance(element) {
  const icon = element && element.closest ? element.closest("svg[" + INSTANCE_ATTR + "]") : null;
  return icon && NODE.test(icon.getAttribute(INSTANCE_ATTR) || "") ? icon : null;
}

function annotated(element) {
  if (!element || !element.getAttribute) return "";
  const nodeRef = element.getAttribute("data-vz-node") || (element.localName === "svg" ? element.getAttribute(INSTANCE_ATTR) : null);
  return nodeRef && NODE.test(nodeRef) ? nodeRef : "";
}

export function nodeElement(target) {
  const element = platformElement(target) || iconInstance(target) || (target && target.closest ? target.closest("[data-vz-node]") : null);
  return element && annotated(element) ? element : null;
}

export function holdsBlocks(element) {
  return Boolean(element) && !HOLDS_NOTHING.has(element.localName) && !element.matches(PLATFORM_SELECTOR) && !element.hasAttribute(INSTANCE_ATTR);
}

export function flowAxis(view, container) {
  const style = view.getComputedStyle(container);
  const display = style.display || "";
  if (display.includes("flex")) return (style.flexDirection || "row").startsWith("row") ? "x" : "y";
  if (display.includes("grid")) {
    const tracks = String(style.gridTemplateColumns || "").trim().split(/\s+/).filter((track) => track && track !== "none");
    return tracks.length > 1 ? "x" : "y";
  }
  return "y";
}

export function slotIndex(boxes, axis, x, y) {
  if (axis === "x") {
    const row = boxes.map((box, index) => ({ box, index })).filter(({ box }) => y >= box.top && y <= box.bottom);
    if (row.length > 0) {
      const before = row.find(({ box }) => x < box.left + box.width / 2);
      return before ? before.index : row[row.length - 1].index + 1;
    }
  }
  const before = boxes.findIndex((box) => y < box.top + box.height / 2);
  return before < 0 ? boxes.length : before;
}

export function dropLine(container, boxes, index, axis) {
  const thick = 3;
  if (boxes.length === 0) return { x: container.x + 4, y: container.y + 4, width: Math.max(0, container.width - 8), height: thick };
  if (axis === "x") {
    const box = index < boxes.length ? boxes[index] : boxes[boxes.length - 1];
    const left = index < boxes.length ? box.left - thick / 2 - (index > 0 && boxes[index - 1].bottom > box.top ? Math.max(0, box.left - boxes[index - 1].right) / 2 : 0) : box.right + thick / 2;
    return { x: left - thick / 2, y: box.top, width: thick, height: box.height };
  }
  const box = index < boxes.length ? boxes[index] : boxes[boxes.length - 1];
  const top = index < boxes.length ? box.top - (index > 0 ? Math.max(0, box.top - boxes[index - 1].bottom) / 2 : 0) : box.bottom + thick;
  return { x: box.left, y: top - thick / 2, width: box.width, height: thick };
}

function familyOf(element) {
  const parent = element.parentElement ? element.parentElement.closest("[data-vz-node]") : null;
  const sibling = (step) => {
    for (let current = element[step]; current; current = current[step]) {
      const nodeRef = annotated(current);
      if (nodeRef) return nodeRef;
    }
    return "";
  };
  return { parent: parent ? annotated(parent) : "", previous: sibling("previousElementSibling"), next: sibling("nextElementSibling") };
}

function inheritedSources(element) {
  const view = element.ownerDocument.defaultView;
  const out = {};
  const own = view.getComputedStyle(element);
  for (const name of INHERITED_STYLES) {
    const value = own.getPropertyValue(name);
    let setter = element;
    while (setter.parentElement && view.getComputedStyle(setter.parentElement).getPropertyValue(name) === value) setter = setter.parentElement;
    const nodeRef = setter === element ? null : setter.getAttribute("data-vz-node");
    if (nodeRef && NODE.test(nodeRef)) out[name] = { node: nodeRef, tag: setter.tagName.toLowerCase() };
  }
  return out;
}

function describeStyles(element, nodeRef) {
  return { type: "vz:styles", node: nodeRef, classes: Array.from(element.classList).filter((name) => CLASS_NAME.test(name)).slice(0, MAX_ITEMS), computed: computedOf(element), rem: rootSize(element), rect: rectOf(element), ...geometryOf(element), inherited: inheritedSources(element) };
}

function compactUrl(value) {
  let out = "";
  for (const char of value) {
    const code = char.codePointAt(0);
    if (code > 32 && code !== 127) out += char;
  }
  return out.toLowerCase();
}

function externalUrl(value) {
  const compact = compactUrl(value);
  if (compact.startsWith("//") || compact.startsWith("\\")) return true;
  const colon = compact.indexOf(":");
  if (colon < 0) return false;
  const slash = compact.search(/[/?#]/);
  return slash < 0 || colon < slash;
}

export function liveAttrRefused(name, value) {
  if (!LIVE_ATTR_NAME.test(name) || name.startsWith("on") || name.startsWith("data-vz-") || FORBIDDEN_LIVE_ATTRS.has(name)) return true;
  if (value === null) return false;
  const compact = compactUrl(value);
  if (URL_ATTRS.has(name) && (compact.startsWith("javascript:") || compact.startsWith("vbscript:") || compact.startsWith("data:"))) return true;
  if (name === "type" && value.trim().toLowerCase() === "password") return true;
  if (name === "autocomplete" && value.toLowerCase().split(/\s+/).some((token) => token.startsWith("cc-") || CREDENTIAL_TOKENS.has(token))) return true;
  return (name === "action" || name === "formaction") && externalUrl(value);
}

export function liveClassRefused(token) {
  return !LIVE_CLASS.test(token);
}

function describe(element, nodeRef, revs) {
  const attributes = {};
  for (const attribute of Array.from(element.attributes).slice(0, MAX_ITEMS)) {
    if (ATTR_NAME.test(attribute.name)) attributes[attribute.name] = clip(attribute.value);
  }
  return {
    type: "vz:select",
    node: nodeRef,
    rect: rectOf(element),
    tag: element.tagName.toLowerCase(),
    text: clip((element.textContent || "").trim()),
    attrs: attributes,
    classes: Array.from(element.classList).filter((name) => CLASS_NAME.test(name)).slice(0, MAX_ITEMS),
    path: elementPath(element),
    rev: revisionOf(revs, nodeRef),
    item: itemOf(element),
    computed: computedOf(element),
    rem: rootSize(element),
    ...geometryOf(element),
    inherited: inheritedSources(element),
    family: familyOf(element),
    ...(element.localName === "svg" && element.hasAttribute(INSTANCE_ATTR) ? { kind: "icon" } : {}),
  };
}

function formatArg(value) {
  if (typeof value === "string") return value;
  if (value instanceof Error) return value.stack || value.message;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function resolveEditorOrigin(win) {
  const ancestors = win.location.ancestorOrigins ? Array.from(win.location.ancestorOrigins) : undefined;
  let origin = editorOriginFrom(win.location.hash, ancestors);
  try {
    if (origin) {
      win.sessionStorage.setItem(ORIGIN_STORAGE_KEY, origin);
      win.history.replaceState(win.history.state, "", win.location.pathname + win.location.search);
    } else {
      origin = editorOriginFrom("#" + EDITOR_ORIGIN_PARAM + "=" + encodeURIComponent(win.sessionStorage.getItem(ORIGIN_STORAGE_KEY) || ""), ancestors);
    }
  } catch {
    return origin;
  }
  return origin;
}

function escapeAttrValue(value) {
  return value.replace(/&/g, "&amp;").replace(/"/g, "&quot;");
}

function inertStyle(node) {
  if (node.nodeType !== 1 || node.localName !== "style") return false;
  const type = (node.getAttribute("type") || "").trim().toLowerCase();
  return type !== "" && type !== "text/css";
}

export function renderable(node, host) {
  return node !== host && !inertStyle(node) && !(node.nodeType === 1 && node.localName === "script");
}

export function serializeDocument(doc, skip) {
  let out = "<!doctype html>";
  const write = (node, raw) => {
    if (node.nodeType === 3 || node.nodeType === 4) {
      out += raw ? node.nodeValue : escapeText(node.nodeValue);
      return;
    }
    if (node.nodeType === 9 || node.nodeType === 11) {
      for (const child of node.childNodes) write(child, false);
      return;
    }
    if (node.nodeType !== 1 || node === skip || inertStyle(node)) return;
    const name = node.localName;
    out += "<" + name;
    for (const attribute of Array.from(node.attributes)) out += " " + attribute.name + '="' + escapeAttrValue(attribute.value) + '"';
    out += ">";
    if (VOID_TAGS.has(name)) return;
    if (node.shadowRoot && node.shadowRoot.mode === "open") {
      out += '<template shadowrootmode="open">';
      for (const child of node.shadowRoot.childNodes) write(child, false);
      out += "</template>";
    }
    if (name === "template" && node.content) {
      for (const child of node.content.childNodes) write(child, false);
    }
    const rawText = RAW_TEXT.has(name);
    for (const child of node.childNodes) write(child, rawText);
    out += "</" + name + ">";
  };
  for (const child of doc.childNodes) write(child, false);
  return out;
}

function startMetrics(win) {
  const perf = { lcp: 0, fcp: 0, cls: 0, session: 0, sessionStart: 0, sessionLast: 0, longTasks: [] };
  const observe = (type, onEntry) => {
    try {
      new win.PerformanceObserver((list) => {
        for (const entry of list.getEntries()) onEntry(entry);
      }).observe({ type, buffered: true });
    } catch {
      return;
    }
  };
  observe("largest-contentful-paint", (entry) => {
    perf.lcp = Math.max(perf.lcp, entry.renderTime || entry.loadTime || entry.startTime);
  });
  observe("paint", (entry) => {
    if (entry.name === "first-contentful-paint") perf.fcp = entry.startTime;
  });
  observe("longtask", (entry) => {
    perf.longTasks.push([entry.startTime, entry.duration]);
  });
  observe("layout-shift", (entry) => {
    if (entry.hadRecentInput) return;
    if (perf.session && entry.startTime - perf.sessionLast < 1000 && entry.startTime - perf.sessionStart < 5000) {
      perf.session += entry.value;
    } else {
      perf.session = entry.value;
      perf.sessionStart = entry.startTime;
    }
    perf.sessionLast = entry.startTime;
    perf.cls = Math.max(perf.cls, perf.session);
  });
  return () => {
    const tbt = perf.longTasks.filter(([start]) => start >= perf.fcp).reduce((sum, [, duration]) => sum + Math.max(0, duration - 50), 0);
    return { lcp: Math.round(perf.lcp), cls: Math.round(perf.cls * 1000) / 1000, tbt: Math.round(tbt) };
  };
}

function createProbe(win, doc) {
  let paint = null;
  const colors = new Map();
  const rgba = (value) => {
    if (colors.has(value)) return colors.get(value);
    if (paint === null) {
      const canvas = doc.createElement("canvas");
      canvas.width = 1;
      canvas.height = 1;
      paint = (canvas.getContext && canvas.getContext("2d", { willReadFrequently: true })) || false;
    }
    if (!paint) return [0, 0, 0, 0];
    paint.clearRect(0, 0, 1, 1);
    paint.fillStyle = "rgba(0, 0, 0, 0)";
    paint.fillStyle = value || "rgba(0, 0, 0, 0)";
    paint.fillRect(0, 0, 1, 1);
    const data = paint.getImageData(0, 0, 1, 1).data;
    const out = [data[0], data[1], data[2], data[3] / 255];
    colors.set(value, out);
    return out;
  };
  const blend = (top, bottom) => {
    const a = top[3];
    return [top[0] * a + bottom[0] * (1 - a), top[1] * a + bottom[1] * (1 - a), top[2] * a + bottom[2] * (1 - a), 1];
  };
  const channel = (value) => Math.min(255, Math.max(0, Math.round(value)));
  const round3 = (c) => [channel(c[0]), channel(c[1]), channel(c[2])];
  const nodeOf = (element) => {
    const owner = element && element.closest ? element.closest("[data-vz-node]") : null;
    const value = owner ? owner.getAttribute("data-vz-node") || "" : "";
    return NODE.test(value) ? value : "";
  };
  const visible = (element) => {
    try {
      return element.checkVisibility({ opacityProperty: true, visibilityProperty: true });
    } catch {
      return true;
    }
  };
  const ancestors = (element) => {
    const out = [];
    for (let at = element; at && at.nodeType === 1; at = at.parentElement) out.push(at);
    return out;
  };
  const frames = (count) =>
    new Promise((resolve) => {
      const step = (left) => (left <= 0 ? resolve() : win.requestAnimationFrame(() => step(left - 1)));
      step(count);
    });
  const sleep = (ms) => new Promise((resolve) => win.setTimeout(resolve, ms));
  const backgroundUnder = (element, x, y) => {
    const inView = x >= 0 && y >= 0 && x < win.innerWidth && y < win.innerHeight;
    let stack = [];
    try {
      stack = inView && doc.elementsFromPoint ? doc.elementsFromPoint(x, y) : [];
    } catch {
      stack = [];
    }
    const start = stack.indexOf(element);
    const chain = start >= 0 ? stack.slice(start) : ancestors(element);
    const layers = [];
    for (const at of chain) {
      if (at !== element && MEDIA_TAGS.has(at.tagName)) return null;
      const style = win.getComputedStyle(at);
      if (style.backgroundImage && style.backgroundImage !== "none") return null;
      const color = rgba(style.backgroundColor);
      if (color[3] > 0) {
        layers.push(color);
        if (color[3] >= 1) break;
      }
    }
    let base = [255, 255, 255, 1];
    for (let i = layers.length - 1; i >= 0; i--) base = blend(layers[i], base);
    return base;
  };
  const sampled = new WeakSet();
  const texts = [];
  const sampleTexts = () => {
    if (!doc.body || texts.length >= MAX_TEXTS) return;
    const walker = doc.createTreeWalker(doc.body, 4);
    const parents = new Set();
    while (walker.nextNode() && parents.size < MAX_SCAN) {
      const current = walker.currentNode;
      const parent = current.parentElement;
      if (parent && !sampled.has(parent) && current.nodeValue.trim()) parents.add(parent);
    }
    for (const element of parents) {
      if (texts.length >= MAX_TEXTS) break;
      if (SKIP_TEXT.has(element.tagName) || element.closest(OVERLAY_TAG)) {
        sampled.add(element);
        continue;
      }
      const box = element.getBoundingClientRect();
      if (box.width < 1 || box.height < 1) continue;
      const x = Math.min(Math.max(box.left + Math.min(box.width, 40) / 2, 0), win.innerWidth - 1);
      const y = box.top + Math.min(box.height / 2, 10);
      if (y < 0 || y >= win.innerHeight) continue;
      sampled.add(element);
      if (!visible(element)) continue;
      const style = win.getComputedStyle(element);
      const fg = rgba(style.color);
      if (fg[3] === 0) continue;
      const bg = backgroundUnder(element, x, y);
      if (!bg) continue;
      texts.push({ node: nodeOf(element), fg: round3(blend(fg, bg)), bg: round3(bg), px: parseFloat(style.fontSize) || 16, weight: parseInt(style.fontWeight, 10) || 400 });
    }
  };
  const pageHeight = () => Math.round(Math.max(doc.documentElement.scrollHeight || 0, doc.body ? doc.body.scrollHeight || 0 : 0, win.innerHeight || 0, 1));
  const scrollTo = async (top) => {
    win.scrollTo({ top, left: 0, behavior: "instant" });
    await frames(2);
  };
  const scrollThrough = async () => {
    const step = Math.max(1, win.innerHeight || 1);
    const steps = Math.min(MAX_VIEWPORTS_DOWN, Math.ceil(pageHeight() / step));
    for (let i = 0; i < steps; i++) {
      await scrollTo(i * step);
      await sleep(SCROLL_SETTLE_MS);
      sampleTexts();
    }
    await scrollTo(0);
  };
  const settled = () => whenPainted(win);
  const escape = win.CSS && win.CSS.escape ? win.CSS.escape : (value) => value.replace(/["\\]/g, "\\$&");
  const locate = (nodeRef) => {
    const element = doc.querySelector('[data-vz-node="' + escape(nodeRef) + '"]');
    if (!element) return null;
    const box = element.getBoundingClientRect();
    return { top: Math.round(box.top + win.scrollY), bottom: Math.round(box.bottom + win.scrollY) };
  };
  const boxes = (limit) => {
    const out = [];
    for (const element of Array.from(doc.querySelectorAll("[data-vz-node]"))) {
      if (out.length >= MAX_BOXES) break;
      const nodeRef = element.getAttribute("data-vz-node");
      if (!NODE.test(nodeRef) || element.closest(OVERLAY_TAG)) continue;
      const box = element.getBoundingClientRect();
      const y = Math.round(box.top + win.scrollY);
      if (box.width < MIN_BOX_WIDTH || box.height < MIN_BOX_HEIGHT || y >= limit || !visible(element)) continue;
      out.push({ node: nodeRef, tag: element.tagName.toLowerCase(), x: Math.round(box.left + win.scrollX), y, w: Math.round(box.width), h: Math.round(box.height) });
    }
    return out;
  };
  const clipped = (element, width) => {
    for (let at = element.parentElement; at && at !== doc.body && at !== doc.documentElement; at = at.parentElement) {
      if (win.getComputedStyle(at).overflowX !== "visible" && at.getBoundingClientRect().right <= width + 1) return true;
    }
    return false;
  };
  const overflow = () => {
    const width = doc.documentElement.clientWidth;
    const scrollWidth = Math.max(0, Math.round(doc.documentElement.scrollWidth || 0));
    if (scrollWidth <= width + 1 || !doc.body) return { scrollWidth, nodes: [] };
    const offenders = [];
    for (const element of Array.from(doc.body.querySelectorAll("*"))) {
      if (offenders.length >= MAX_OFFENDERS) break;
      const box = element.getBoundingClientRect();
      if (box.width > 0 && box.right > width + 1 && !clipped(element, width)) offenders.push(element);
    }
    const leaves = offenders.filter((element) => !offenders.some((other) => other !== element && element.contains(other)));
    const nodes = [];
    for (const element of leaves) {
      const nodeRef = nodeOf(element);
      if (nodeRef && !nodes.includes(nodeRef)) nodes.push(nodeRef);
      if (nodes.length === MAX_OVERFLOW) break;
    }
    return { scrollWidth, nodes };
  };
  const images = () => {
    const out = [];
    for (const img of Array.from(doc.images || [])) {
      if (out.length >= MAX_IMAGES) break;
      const box = img.getBoundingClientRect();
      if (!img.complete || !img.naturalWidth || box.width < 1 || box.height < 1) continue;
      const src = img.currentSrc || img.src || "";
      if (/\.svg(\?|#|$)/i.test(src) || src.startsWith("data:image/svg")) continue;
      const fit = win.getComputedStyle(img).objectFit;
      const sx = box.width / img.naturalWidth;
      const sy = box.height / img.naturalHeight;
      const scale = fit === "cover" ? Math.max(sx, sy) : fit === "contain" || fit === "scale-down" ? Math.min(sx, sy) : sx;
      out.push({ node: nodeOf(img), naturalW: img.naturalWidth, naturalH: img.naturalHeight, shownW: Math.round(img.naturalWidth * scale), shownH: Math.round(img.naturalHeight * scale) });
    }
    return out;
  };
  const network = () => {
    let entries = [];
    try {
      entries = [...win.performance.getEntriesByType("navigation"), ...win.performance.getEntriesByType("resource")];
    } catch {
      entries = [];
    }
    const failedRequests = [];
    let transferBytes = 0;
    for (const entry of entries) {
      transferBytes += Number.isFinite(entry.transferSize) && entry.transferSize > 0 ? entry.transferSize : 0;
      if (failedRequests.length < MAX_ERRORS && Number.isInteger(entry.responseStatus) && entry.responseStatus >= 400 && entry.responseStatus <= 599) {
        failedRequests.push({ url: String(entry.name || "").slice(0, MAX_URL), status: entry.responseStatus });
      }
    }
    return { failedRequests, transferBytes: Math.round(transferBytes) };
  };
  return { sampleTexts, texts, pageHeight, scrollTo, scrollThrough, settled, locate, boxes, overflow, images, network };
}

async function captureImage(win, doc, host, probe, metrics, errors, load, request) {
  const { domToCanvas } = await load();
  const fixed = Number.isInteger(request.height);
  if (fixed) {
    await probe.scrollTo(0);
    probe.sampleTexts();
  } else {
    await probe.scrollThrough();
  }
  await probe.settled();
  const focus = request.focus ? probe.locate(request.focus) : null;
  if (request.focus && !focus) return { failure: "focus_missing" };
  const width = Math.max(1, Math.round(doc.documentElement.clientWidth || win.innerWidth || 1));
  const pageHeight = probe.pageHeight();
  const capturedHeight = fixed ? request.height : Math.min(pageHeight, MAX_VIEWPORTS_DOWN * Math.max(1, win.innerHeight || 1));
  const scale = fixed ? 1 : Math.min(1, Math.sqrt(MAX_CAPTURE_PIXELS / (width * capturedHeight)));
  const canvas = await domToCanvas(doc.documentElement, { width, height: capturedHeight, scale, backgroundColor: "#ffffff", filter: (candidate) => renderable(candidate, host) });
  const image = canvas.toDataURL("image/jpeg", JPEG_QUALITY);
  if (image.length > MAX_IMAGE) return { failure: "too_large" };
  if (!JPEG_URL.test(image)) return { failure: "capture_failed" };
  const spill = probe.overflow();
  const traffic = probe.network();
  return {
    payload: {
      kind: "image",
      image,
      width,
      pageHeight,
      capturedHeight,
      scale,
      truncated: !fixed && capturedHeight < pageHeight,
      boxes: probe.boxes(capturedHeight),
      focus,
      facts: {
        scrollWidth: spill.scrollWidth,
        overflow: spill.nodes,
        texts: probe.texts.slice(0, MAX_TEXTS),
        images: probe.images(),
        metrics: metrics(),
        consoleErrors: errors.slice(0, MAX_ERRORS),
        failedRequests: traffic.failedRequests,
        transferBytes: traffic.transferBytes,
      },
    },
  };
}

const OVERLAY_STYLE = `
:host{all:initial}
.box{position:fixed;left:0;top:0;pointer-events:none;box-sizing:border-box;border-radius:2px;z-index:2147483646}
.drop-line{position:fixed;left:0;top:0;display:none;pointer-events:none;border-radius:2px;background:#2563eb;box-shadow:0 0 0 1px #fff;z-index:2147483647}
.drop-box{position:fixed;left:0;top:0;display:none;pointer-events:none;box-sizing:border-box;border-radius:3px;outline:1px dashed #2563eb;z-index:2147483646}
.drop-box.refused{outline:2px solid #dc2626;background:rgba(220,38,38,.06)}
.selection{outline:2px solid #2563eb}
.hover{outline:1px dashed #2563eb}
.repeat{outline:1px dotted #93c5fd}
.elo{outline:2px solid #a855f7;background:rgba(168,85,247,.08)}
.bar{position:fixed;display:none;gap:2px;padding:3px;background:#111827;border-radius:6px;z-index:2147483647;font:13px system-ui,sans-serif}
.bar.open{display:flex}
.bar button{all:unset;color:#fff;padding:2px 8px;border-radius:4px;cursor:pointer}
.bar button:focus-visible,.bar button:hover{background:#374151}
.bar input{display:none;width:220px;padding:2px 6px;border:0;border-radius:4px;font:inherit}
.bar.link input{display:block}
`;

const loadScreenshotModule = () => import("./vz-screenshot.js");

export function startBridge(win, parent = win.parent, options = {}) {
  const doc = win.document;
  const editorOrigin = resolveEditorOrigin(win);
  const revsMeta = doc.querySelector('meta[name="vz-revs"]');
  const revs = revisionsFrom(revsMeta ? revsMeta.getAttribute("content") : null);
  if (!editorOrigin) return;
  const loadScreenshot = options.loadScreenshot || loadScreenshotModule;
  const metrics = startMetrics(win);
  const errors = [];
  const noteError = (message) => {
    if (errors.length < MAX_ERRORS) errors.push(String(message).slice(0, MAX_ERROR_TEXT));
  };
  const send = (message) => {
    if (decodeFrameMessage(message)) parent.postMessage(message, editorOrigin);
  };
  const host = doc.createElement(OVERLAY_TAG);
  const shadow = host.attachShadow({ mode: "open" });
  const sheet = new win.CSSStyleSheet();
  sheet.replaceSync(OVERLAY_STYLE);
  shadow.adoptedStyleSheets = [sheet];
  shadow.innerHTML = '<div class="boxes"></div><div class="drop-box"></div><div class="drop-line"></div><div class="bar" role="toolbar"><button type="button" data-cmd="bold"><b>B</b></button><button type="button" data-cmd="italic"><i>I</i></button><button type="button" data-cmd="link"></button><input type="url" placeholder="https://"></div>';
  const boxes = shadow.querySelector(".boxes");
  const dropBox = shadow.querySelector(".drop-box");
  const dropMark = shadow.querySelector(".drop-line");
  const bar = shadow.querySelector(".bar");
  const linkInput = bar.querySelector("input");
  const label = (locale) => {
    const words = TOOLBAR_LABELS[locale] || TOOLBAR_LABELS.pt;
    bar.querySelectorAll("button").forEach((button, index) => {
      button.setAttribute("aria-label", words[index]);
      button.setAttribute("title", words[index]);
    });
    bar.querySelector('[data-cmd="link"]').textContent = words[2];
    linkInput.setAttribute("aria-label", words[3]);
  };
  label("pt");
  const fromOverlay = (event) => event.target === host || (event.composedPath ? event.composedPath().includes(host) : false);
  doc.documentElement.appendChild(host);

  let mode = "preview";
  let hovered = null;
  let picked = null;
  let highlights = [];
  let editing = null;
  let savedRange = null;
  let frame = 0;
  let scrolled = false;
  let slot = null;
  let press = null;
  let dragging = null;
  let swallowClick = false;
  let edgeScroll = 0;
  let slotFromPointer = false;
  let showHover = false;
  let sentScroll = win.scrollY;
  let guardFrame = 0;
  let pointer = null;
  const pool = [];
  const placed = [];

  const SETTLE_FRAMES = 30;
  let followed = "";
  let settling = 0;
  const placeOf = (element) => {
    const box = element.getBoundingClientRect();
    return [box.left + win.scrollX, box.top + win.scrollY, box.width, box.height].map((value) => Math.round(value * 2) / 2).join(",");
  };
  const watcher = win.ResizeObserver ? new win.ResizeObserver(() => schedule()) : null;
  let watched = null;
  const follow = () => {
    if (watched !== picked) {
      if (watcher && watched) watcher.unobserve(watched);
      watched = picked;
      if (watcher && watched) watcher.observe(watched);
    }
    if (settling > 0) {
      settling -= 1;
      schedule();
    }
    if (!picked || !picked.isConnected || editing) return;
    if (placeOf(picked) !== followed) restyle();
  };
  const announce = (element, nodeRef) => {
    syncScroll();
    followed = placeOf(element);
    send(describe(element, nodeRef, revs));
    schedule();
  };
  const settle = () => {
    settling = SETTLE_FRAMES;
    schedule();
  };
  const nodeOf = nodeElement;
  const escape = win.CSS && win.CSS.escape ? win.CSS.escape : (value) => value.replace(/["\\]/g, "\\$&");
  const instances = (nodeRef) => {
    const value = escape(nodeRef);
    return Array.from(doc.querySelectorAll('[data-vz-node="' + value + '"], svg[' + INSTANCE_ATTR + '="' + value + '"]')).slice(0, MAX_ITEMS);
  };
  const byNode = (nodeRef, item, path) => {
    const all = instances(nodeRef);
    const wantsItem = Number.isInteger(item);
    const wanted = Array.isArray(path) ? path.join("/") : null;
    const fits = (element) => (!wantsItem || itemOf(element) === item) && (wanted === null || elementPath(element).join("/") === wanted);
    if (picked && picked.isConnected && annotated(picked) === nodeRef && fits(picked)) return picked;
    const candidates = wantsItem ? all.filter((element) => itemOf(element) === item) : all;
    return candidates.find(fits) || candidates[0] || all[0] || null;
  };

  const holderOf = (element) => {
    for (let at = element.parentElement; at; at = at.parentElement) {
      if (at === doc.body || at === doc.documentElement) return doc.body;
      if (annotated(at) && holdsBlocks(at)) return at;
    }
    return doc.body;
  };
  const slotChildren = (container, moving) =>
    Array.from(container.children).filter((child) => child !== moving && child !== host && annotated(child) && win.getComputedStyle(child).display !== "none");
  const nearEdge = (element, x, y) => {
    if (element === doc.body) return false;
    const box = element.getBoundingClientRect();
    const band = Math.min(EDGE_BAND, box.height / 4, box.width / 4);
    return flowAxis(win, holderOf(element)) === "x" ? x - box.left < band || box.right - x < band : y - box.top < band || box.bottom - y < band;
  };
  const containerAt = (x, y, moving) => {
    let stack = [];
    try {
      stack = doc.elementsFromPoint ? doc.elementsFromPoint(x, y) : [];
    } catch {
      stack = [];
    }
    const hit = stack.find((element) => element !== host && element !== doc.documentElement);
    if (hit && moving && (hit === moving || moving.contains(hit))) return { container: moving, refused: true };
    let container = doc.body;
    for (let at = hit || null; at && at !== doc.body && at !== doc.documentElement; at = at.parentElement) {
      if (annotated(at) && holdsBlocks(at)) {
        container = at;
        break;
      }
    }
    while (container !== doc.body && (nearEdge(container, x, y) || (moving && container.parentElement === moving.parentElement && container.localName === moving.localName))) container = holderOf(container);
    return { container, refused: false };
  };
  const slotAt = (x, y, moving) => {
    const found = containerAt(x, y, moving);
    if (found.refused) return { container: found.container, children: [], index: 0, axis: "y", refused: true, moving };
    const children = slotChildren(found.container, moving);
    const axis = flowAxis(win, found.container);
    const index = slotIndex(children.map((child) => child.getBoundingClientRect()), axis, x, y);
    return { container: found.container, children, index, axis, refused: false, moving };
  };
  const slotOf = (current) => {
    const container = current.container === doc.body ? "" : annotated(current.container);
    const tag = current.container.localName;
    if (current.refused) return { container, tag, anchor: container, place: "into", axis: current.axis, refused: "inside_self" };
    const { children, index } = current;
    if (children.length === 0) return { container, tag, anchor: container, place: "into", axis: current.axis };
    return index < children.length
      ? { container, tag, anchor: annotated(children[index]), place: "before", axis: current.axis }
      : { container, tag, anchor: annotated(children[children.length - 1]), place: "after", axis: current.axis };
  };
  const showSlot = (next, point) => {
    slot = next;
    send({ type: "vz:drop", slot: next ? slotOf(next) : null, ...(point ? { x: point.x, y: point.y } : {}) });
    schedule();
  };
  const stopEdgeScroll = () => {
    if (edgeScroll) win.cancelAnimationFrame(edgeScroll);
    edgeScroll = 0;
  };
  const probeAt = (x, y, moving, point) => {
    showSlot(slotAt(x, y, moving), point);
    stopEdgeScroll();
    const step = y < SCROLL_BAND ? -SCROLL_STEP : y > win.innerHeight - SCROLL_BAND ? SCROLL_STEP : 0;
    if (step === 0) return;
    const tick = () => {
      edgeScroll = 0;
      const before = win.scrollY;
      win.scrollBy(0, step);
      if (win.scrollY === before) return;
      showSlot(slotAt(x, y, moving), point);
      edgeScroll = win.requestAnimationFrame(tick);
    };
    edgeScroll = win.requestAnimationFrame(tick);
  };
  const clearSlot = () => {
    stopEdgeScroll();
    slot = null;
    slotFromPointer = false;
    schedule();
  };
  const syncScroll = () => {
    if (win.scrollY === sentScroll) return;
    sentScroll = win.scrollY;
    send({ type: "vz:scroll", y: sentScroll });
  };
  const designing = () => {
    doc.documentElement.toggleAttribute(DESIGN_ATTR, mode !== "preview" && !showHover);
  };
  const guardSoon = () => {
    if (!guardFrame) guardFrame = win.requestAnimationFrame(() => {
      guardFrame = 0;
      guardHoverSheets(doc);
    });
  };
  const startingSlot = (moving) => {
    const from = moving || (picked && picked.isConnected ? picked : null);
    if (!from) {
      const children = slotChildren(doc.body, null);
      return { container: doc.body, children, index: children.length, axis: "y", refused: false, moving };
    }
    const container = holderOf(from);
    const children = slotChildren(container, moving);
    const before = Array.from(container.children).filter((child) => children.includes(child) && child.compareDocumentPosition(from) & 4).length;
    return { container, children, index: moving ? before : Math.min(children.length, before + 1), axis: flowAxis(win, container), refused: false, moving };
  };
  const stepSlot = (direction, moving) => {
    const known = slot && !slot.refused && slot.moving === moving && slot.container.isConnected;
    const current = known ? slot : startingSlot(moving);
    let next = current;
    if (!known) direction = "";

    if (direction === "previous") next = { ...current, index: Math.max(0, current.index - 1) };
    if (direction === "next") next = { ...current, index: Math.min(current.children.length, current.index + 1) };
    if (direction === "out" && current.container !== doc.body) {
      const container = holderOf(current.container);
      const children = slotChildren(container, moving);
      next = { container, children, index: children.indexOf(current.container) + 1, axis: flowAxis(win, container), refused: false, moving };
    }
    if (direction === "in") {
      const inner = current.children[current.index] || current.children[current.index - 1];
      if (inner && holdsBlocks(inner) && inner !== moving) next = { container: inner, children: slotChildren(inner, moving), index: 0, axis: flowAxis(win, inner), refused: false, moving };
    }
    showSlot(next, null);
    const near = next.children[next.index] || next.children[next.children.length - 1] || next.container;
    if (near && near !== doc.body && near.scrollIntoView) near.scrollIntoView({ block: "nearest" });
  };

  const placeBox = (index, kind, box) => {
    let div = pool[index];
    if (!div) {
      div = doc.createElement("div");
      boxes.appendChild(div);
      pool.push(div);
      placed.push("");
    }
    const className = "box " + kind;
    if (div.className !== className) div.className = className;
    const css = "transform:translate(" + box.left + "px," + box.top + "px);width:" + box.width + "px;height:" + box.height + "px";
    if (placed[index] !== css) {
      placed[index] = css;
      div.style.cssText = css;
    }
  };
  const placeMark = (element, rect, className) => {
    if (!rect) {
      if (element.style.display !== "none") element.style.display = "none";
      return;
    }
    if (element.className !== className) element.className = className;
    element.style.cssText = "display:block;transform:translate(" + rect.x + "px," + rect.y + "px);width:" + rect.width + "px;height:" + rect.height + "px";
  };

  const paint = () => {
    frame = 0;
    follow();
    const wanted = [];
    for (const item of highlights) {
      const element = byNode(item.node, item.item);
      if (!element) continue;
      if (item.kind === "selection") {
        for (const other of instances(item.node)) if (other !== element) wanted.push([other, "repeat"]);
      }
      wanted.push([element, item.kind]);
    }
    if (hovered && hovered.isConnected && mode === "select" && !dragging) wanted.push([hovered, "hover"]);
    const rects = wanted.map(([element]) => element.getBoundingClientRect());
    const live = slot && slot.container.isConnected ? slot : null;
    const area = live && live.container !== doc.body ? rectOf(live.container) : null;
    const frameBox = { x: 0, y: 0, width: win.innerWidth, height: 0 };
    const line = live && !live.refused ? dropLine(live.container === doc.body ? frameBox : contentBox(live.container), live.children.map((child) => child.getBoundingClientRect()), live.index, live.axis) : null;
    const editBox = editing ? editing.element.getBoundingClientRect() : null;
    for (let i = 0; i < wanted.length; i++) placeBox(i, wanted[i][1], rects[i]);
    while (pool.length > wanted.length) {
      pool.pop().remove();
      placed.pop();
    }
    placeMark(dropBox, area, live && live.refused ? "drop-box refused" : "drop-box");
    placeMark(dropMark, line, "drop-line");
    if (editBox) {
      bar.style.left = Math.max(4, editBox.left) + "px";
      bar.style.top = Math.max(4, editBox.top - 36) + "px";
    }
    if (scrolled) {
      scrolled = false;
      syncScroll();
      rehover();
    }
  };
  const schedule = () => {
    if (!frame) frame = win.requestAnimationFrame(paint);
  };

  const parseHtml = (html) => {
    const template = doc.createElement("template");
    template.innerHTML = html;
    return template.content;
  };
  const finishEdit = (commit) => {
    if (!editing) return;
    const { element, original, nodeRef } = editing;
    editing = null;
    bar.className = "bar";
    element.removeAttribute("contenteditable");
    if (!commit) {
      element.innerHTML = original;
      send({ type: "vz:text-cancel", node: nodeRef });
      return;
    }
    const before = sanitizeInline(parseHtml(original)).html;
    const after = sanitizeInline(element);
    element.innerHTML = after.html;
    send(after.html === before ? { type: "vz:text-cancel", node: nodeRef } : { type: "vz:text-commit", node: nodeRef, html: after.html, text: after.text, rev: revisionOf(revs, nodeRef), item: itemOf(element) });
    schedule();
  };
  const beginEdit = (element) => {
    if (!element || editing || platformElement(element) || !isInlineOnly(element)) return;
    const nodeRef = annotated(element);
    editing = { element, original: element.innerHTML, nodeRef };
    element.setAttribute("contenteditable", "true");
    element.focus();
    const selection = win.getSelection();
    const range = doc.createRange();
    range.selectNodeContents(element);
    selection.removeAllRanges();
    selection.addRange(range);
    bar.className = "bar open";
    send({ type: "vz:text-begin", node: nodeRef });
    schedule();
  };
  const restoreRange = () => {
    if (!savedRange) return;
    const selection = win.getSelection();
    selection.removeAllRanges();
    selection.addRange(savedRange);
  };

  bar.addEventListener("mousedown", (event) => {
    if (event.target !== linkInput) event.preventDefault();
  });
  bar.addEventListener("click", (event) => {
    const button = event.target.closest("button");
    if (!button || !editing) return;
    const command = button.getAttribute("data-cmd");
    if (command !== "link") {
      doc.execCommand(command);
      return;
    }
    const selection = win.getSelection();
    savedRange = selection.rangeCount ? selection.getRangeAt(0).cloneRange() : null;
    const anchor = savedRange && savedRange.startContainer.parentElement ? savedRange.startContainer.parentElement.closest("a") : null;
    linkInput.value = anchor ? anchor.getAttribute("href") || "" : "";
    bar.className = "bar open link";
    linkInput.focus();
  });
  linkInput.addEventListener("keydown", (event) => {
    if (event.key !== "Enter" && event.key !== "Escape") return;
    event.preventDefault();
    const element = editing && editing.element;
    if (!element) return;
    element.focus();
    restoreRange();
    if (event.key === "Enter") {
      const href = safeHref(linkInput.value);
      if (href === null) doc.execCommand("unlink");
      else doc.execCommand("createLink", false, href);
    }
    bar.className = "bar open";
  });

  doc.addEventListener("keydown", (event) => {
    if (!editing || event.target !== editing.element) return;
    if (event.key === "Enter") {
      event.preventDefault();
      finishEdit(true);
    } else if (event.key === "Escape") {
      event.preventDefault();
      finishEdit(false);
    } else if ((event.ctrlKey || event.metaKey) && (event.key === "b" || event.key === "i")) {
      event.preventDefault();
      doc.execCommand(event.key === "b" ? "bold" : "italic");
    }
  }, true);
  doc.addEventListener("keydown", (event) => {
    if (!(event.ctrlKey || event.metaKey) || event.shiftKey || event.altKey || String(event.key).toLowerCase() !== "s") return;
    event.preventDefault();
    if (editing) finishEdit(true);
    send({ type: "vz:save" });
  }, true);
  doc.addEventListener("paste", (event) => {
    if (!editing || !editing.element.contains(event.target)) return;
    event.preventDefault();
    const pasted = (event.clipboardData ? event.clipboardData.getData("text/plain") : "").replace(/\s+/g, " ");
    doc.execCommand("insertText", false, pasted);
  }, true);
  doc.addEventListener("drop", (event) => {
    if (editing && editing.element.contains(event.target)) event.preventDefault();
  }, true);
  doc.addEventListener("focusout", (event) => {
    if (editing && event.target === editing.element && !(event.relatedTarget && (event.relatedTarget === host || shadow.contains(event.relatedTarget)))) finishEdit(true);
  }, true);

  const hoverOn = (element) => {
    if (element === hovered) return;
    hovered = element;
    if (element) send({ type: "vz:hover", node: annotated(element), rect: rectOf(element), rev: revisionOf(revs, annotated(element)) });
    schedule();
  };
  const rehover = () => {
    if (!pointer || mode !== "select" || editing || dragging || !doc.elementFromPoint) return;
    const under = doc.elementFromPoint(pointer.x, pointer.y);
    if (under && (under === host || host.contains(under))) return;
    hoverOn(under ? nodeOf(under) : null);
  };
  const cancelPress = () => {

    if (press) win.clearTimeout(press.timer);
    press = null;
  };
  const finishDrag = (commit) => {
    if (!dragging) return;
    dragging = null;
    host.style.cursor = "";
    clearSlot();
    send({ type: "vz:drag-end", commit });
  };
  const beginDrag = () => {
    const begun = press;
    press = null;
    if (!begun || !begun.element.isConnected || mode !== "select" || editing) return;
    const nodeRef = annotated(begun.element);
    dragging = { element: begun.element };
    hovered = null;
    send({ type: "vz:drag-begin", node: nodeRef, item: itemOf(begun.element) });
    probeAt(begun.x, begun.y, begun.element, { x: begun.x, y: begun.y });
  };
  doc.addEventListener("pointerdown", (event) => {
    if (mode !== "select" || editing || dragging || fromOverlay(event) || event.button !== 0) return;
    const element = nodeOf(event.target);
    if (!element || element !== picked) return;
    cancelPress();
    press = { element, x: event.clientX, y: event.clientY, timer: win.setTimeout(beginDrag, LONG_PRESS_MS) };
  }, true);
  doc.addEventListener("pointerup", () => {
    cancelPress();
    if (!dragging) return;
    swallowClick = true;
    finishDrag(true);
  }, true);
  doc.addEventListener("pointercancel", () => {
    cancelPress();
    finishDrag(false);
  }, true);
  doc.addEventListener("keydown", (event) => {
    if (event.key !== "Escape" || !dragging) return;
    event.preventDefault();
    event.stopPropagation();
    finishDrag(false);
  }, true);
  doc.addEventListener("keydown", (event) => {
    if (mode !== "select" || editing || dragging || !(event.ctrlKey || event.metaKey) || event.shiftKey || event.altKey) return;
    const action = { c: "copy", v: "paste", d: "duplicate" }[String(event.key).toLowerCase()];
    if (!action) return;
    event.preventDefault();
    send({ type: "vz:shortcut", action });
  }, true);
  doc.addEventListener("pointermove", (event) => {
    if (slotFromPointer && !dragging) clearSlot();
    if (press && Math.hypot(event.clientX - press.x, event.clientY - press.y) > PRESS_SLOP) cancelPress();
    if (dragging) {
      probeAt(event.clientX, event.clientY, dragging.element, { x: event.clientX, y: event.clientY });
      return;
    }
    pointer = { x: event.clientX, y: event.clientY };
    if (mode !== "select" || editing || fromOverlay(event)) return;
    hoverOn(nodeOf(event.target));
  }, true);
  doc.addEventListener("click", (event) => {
    if (swallowClick) {
      swallowClick = false;
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    if (mode === "preview" || fromOverlay(event) || (editing && editing.element.contains(event.target))) return;
    event.preventDefault();
    event.stopPropagation();
    const element = nodeOf(event.target);
    if (!element) return;
    const nodeRef = annotated(element);
    picked = element;
    if (mode === "comment") {
      send({ type: "vz:comment-pin", node: nodeRef, rect: rectOf(element), point: pinPoint(element.getBoundingClientRect(), event.clientX, event.clientY) });
      return;
    }
    announce(element, nodeRef);
  }, true);
  const askMenu = (element, x, y) => {
    if (element) {
      picked = element;
      announce(element, annotated(element));
    }
    send({ type: "vz:context", node: element ? annotated(element) : "", x, y });
  };
  doc.addEventListener("contextmenu", (event) => {
    if (mode !== "select" || editing || fromOverlay(event)) return;
    event.preventDefault();
    event.stopPropagation();
    askMenu(nodeOf(event.target), event.clientX, event.clientY);
  }, true);
  doc.addEventListener("keydown", (event) => {
    if (mode !== "select" || editing || !(event.key === "ContextMenu" || (event.key === "F10" && event.shiftKey))) return;
    if (!picked || !picked.isConnected) return;
    event.preventDefault();
    const box = picked.getBoundingClientRect();
    askMenu(picked, box.left, box.bottom);
  }, true);
  doc.addEventListener("dblclick", (event) => {
    if (mode !== "select" || fromOverlay(event)) return;
    event.preventDefault();
    beginEdit(nodeOf(event.target));
  }, true);
  doc.addEventListener("submit", (event) => {
    if (mode !== "preview") event.preventDefault();
  }, true);

  win.addEventListener("scroll", () => {
    scrolled = true;
    schedule();
  }, { passive: true });
  win.addEventListener("resize", schedule);
  win.addEventListener("error", (event) => {
    noteError(event.message || "error");
    send({ type: "vz:error", message: clip(event.message || "error"), source: clip(event.filename || ""), line: Number.isInteger(event.lineno) && event.lineno >= 0 ? event.lineno : 0 });
  });
  win.addEventListener("unhandledrejection", (event) => {
    const reason = event.reason;
    noteError(reason && reason.message ? reason.message : formatArg(reason));
    send({ type: "vz:error", message: clip(reason && reason.message ? reason.message : formatArg(reason)), source: "", line: 0 });
  });
  for (const level of ["log", "info", "warn", "error", "debug"]) {
    const original = win.console[level];
    win.console[level] = (...args) => {
      original.apply(win.console, args);
      if (level === "error") noteError(args.map(formatArg).join(" "));
      send({ type: "vz:console", level, message: clip(args.map(formatArg).join(" ")) });
    };
  }

  const capture = async (message) => {
    const fail = (reason) => send({ type: "vz:capture-failed", id: message.id, reason });
    try {
      if (message.kind === "document") {
        const html = serializeDocument(doc, host);
        if (html.length > MAX_DOCUMENT) return fail("too_large");
        return send({ type: "vz:captured", id: message.id, kind: "document", html });
      }
      const outcome = await captureImage(win, doc, host, createProbe(win, doc), metrics, errors, loadScreenshot, message);
      if (outcome.failure) return fail(outcome.failure);
      return send({ type: "vz:captured", id: message.id, ...outcome.payload });
    } catch {
      return fail("capture_failed");
    }
  };

  const applied = new Map();
  const restyle = () => {
    if (!picked || !picked.isConnected) return;
    syncScroll();
    followed = placeOf(picked);
    send(describeStyles(picked, annotated(picked)));
  };
  const undo = (id) => {
    const revert = applied.get(id);
    if (!revert) return;
    applied.delete(id);
    revert();
  };
  const refusalOf = (element, change) => {
    if (!element) return "node_unknown";
    if (change.kind === "classes") return [...change.add, ...change.remove].some(liveClassRefused) ? "class_invalid" : null;
    if (change.kind === "attr") return liveAttrRefused(change.name, change.value) ? "attribute_forbidden" : null;
    if (change.kind === "duplicate") return element.matches(FORBIDDEN_LIVE_TAGS) || element.querySelector(FORBIDDEN_LIVE_TAGS) ? "element_forbidden" : null;
    if (change.kind === "move") {
      const anchor = anchorFor(element, change, true);
      if (!anchor) return "node_unknown";
      if (anchor === element || element.contains(anchor)) return "element_forbidden";
      return change.place === "into" && !holdsBlocks(anchor) ? "element_forbidden" : null;
    }
    return null;
  };
  const anchorFor = (element, change, primary) => {
    const candidates = instances(change.anchor);
    const paired = candidates.find((candidate) => (change.place === "into" ? itemOf(candidate) === itemOf(element) && !element.contains(candidate) : candidate.parentElement === element.parentElement));
    return paired || (primary ? byNode(change.anchor) : null);
  };
  const sharedTargets = (element, message) => {
    const copies = instances(message.node).filter((other) => other !== element);
    if (message.change.kind !== "move") return [element, ...copies];
    return [element, ...copies.filter((other) => anchorFor(other, message.change, false))];
  };
  const perform = (element, change, primary) => {
    if (change.kind === "classes") {
      const added = change.add.filter((name) => !element.classList.contains(name));
      const removed = change.remove.filter((name) => element.classList.contains(name) && !change.add.includes(name));
      removed.forEach((name) => element.classList.remove(name));
      added.forEach((name) => element.classList.add(name));
      return () => {
        added.forEach((name) => element.classList.remove(name));
        removed.forEach((name) => element.classList.add(name));
      };
    }
    if (change.kind === "attr") {
      const before = element.getAttribute(change.name);
      if (change.value === null) element.removeAttribute(change.name);
      else element.setAttribute(change.name, change.value);
      return () => (before === null ? element.removeAttribute(change.name) : element.setAttribute(change.name, before));
    }
    if (change.kind === "remove") {
      const before = element.getAttribute("style");
      element.style.setProperty("display", "none", "important");
      return () => (before === null ? element.removeAttribute("style") : element.setAttribute("style", before));
    }
    if (change.kind === "duplicate") {
      const copy = element.cloneNode(true);
      element.after(copy);
      return () => copy.remove();
    }
    const anchor = anchorFor(element, change, primary);
    const parentNode = element.parentNode;
    const next = element.nextSibling;
    if (change.place === "before") anchor.before(element);
    else if (change.place === "after") anchor.after(element);
    else anchor.appendChild(element);
    return () => parentNode.insertBefore(element, next);
  };
  const apply = (message) => {
    const element = byNode(message.node, message.item);
    const refusal = refusalOf(element, message.change);
    if (refusal) {
      send({ type: "vz:apply-refused", id: message.id, reason: refusal });
      return;
    }
    undo(message.id);
    const reverts = sharedTargets(element, message).map((target, index) => perform(target, message.change, index === 0));
    applied.set(message.id, () => reverts.reverse().forEach((revert) => revert()));
    send({ type: "vz:applied", id: message.id });
    restyle();
    settle();
  };

  win.addEventListener("message", (event) => {
    const message = acceptEditorMessage(event, editorOrigin, parent);
    if (!message) return;
    if (message.type === "vz:capture") {
      capture(message);
      return;
    }
    if (message.type === "vz:apply") {
      apply(message);
      schedule();
      return;
    }
    if (message.type === "vz:revert") {
      undo(message.id);
      restyle();
      settle();
      schedule();
      return;
    }
    if (message.type === "vz:mode") {
      if (message.mode !== "select") finishEdit(true);
      mode = message.mode;
      if (message.locale) label(message.locale);
      hovered = null;
      designing();
      guardSoon();
    } else if (message.type === "vz:highlight") {
      highlights = message.nodes;
    } else if (message.type === "vz:probe") {
      probeAt(message.x, message.y, message.moving ? byNode(message.moving, message.item) : null, null);
      slotFromPointer = true;
      return;
    } else if (message.type === "vz:hover-states") {
      showHover = message.on;
      designing();
      return;
    } else if (message.type === "vz:probe-step") {
      stepSlot(message.direction, message.moving ? byNode(message.moving, message.item) : null);
      return;
    } else if (message.type === "vz:probe-end") {
      clearSlot();
      return;
    } else if (message.type === "vz:select-node") {
      const element = byNode(message.node, message.item, message.path);
      if (element) {
        picked = element;
        announce(element, message.node);
      }
    } else if (message.type === "vz:begin-text-edit") {
      beginEdit(byNode(message.node));
    } else if (message.type === "vz:scroll-y") {
      win.scrollTo({ top: message.y, left: 0, behavior: "instant" });
      whenPainted(win).then(() => {
        const anchor = message.anchor ? byNode(message.anchor.node, message.anchor.item, message.anchor.path) : null;
        const drift = anchor ? anchor.getBoundingClientRect().top - message.anchor.top : 0;
        if (Math.abs(drift) >= 0.5) win.scrollTo({ top: win.scrollY + drift, left: 0, behavior: "instant" });
        sentScroll = win.scrollY;
        send({ type: "vz:scroll", y: win.scrollY });
      });
    } else if (message.type === "vz:scroll-to") {
      const element = byNode(message.node);
      const reduce = win.matchMedia && win.matchMedia("(prefers-reduced-motion: reduce)").matches;
      if (element) element.scrollIntoView({ block: "center", behavior: reduce ? "auto" : "smooth" });
    }
    schedule();
  });

  doc.addEventListener("keydown", (event) => {
    if (event.key !== "Alt" || showHover) return;
    showHover = true;
    designing();
  }, true);
  doc.addEventListener("keyup", (event) => {
    if (event.key !== "Alt" || !showHover) return;
    showHover = false;
    designing();
  }, true);
  win.addEventListener("blur", () => {
    if (!showHover) return;
    showHover = false;
    designing();
  });
  if (win.MutationObserver) {
    new win.MutationObserver((records) => {
      if (records.some((record) => {
        const target = record.target.nodeType === 1 ? record.target : record.target.parentElement;
        return (target && (target.localName === "style" || target.localName === "link")) || Array.from(record.addedNodes).some((added) => added.localName === "style" || added.localName === "link");
      })) guardSoon();
    }).observe(doc.documentElement, { childList: true, subtree: true, characterData: true });
  }
  doc.addEventListener("load", (event) => {
    if (event.target && event.target.localName === "link") guardSoon();
  }, true);
  guardSoon();
  whenPainted(win).then(() => send({ type: "vz:ready" }));

}

if (typeof window !== "undefined" && window.document && window.parent && window.parent !== window) startBridge(window, window.parent);

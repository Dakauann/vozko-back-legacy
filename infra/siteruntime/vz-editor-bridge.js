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
  "vz:hover": { node, rect, rev: optionalCount },
  "vz:select": { node, rect, tag: matching(TAG), text, attrs, classes: listOf(matching(CLASS_NAME)), path: listOf(matching(PATH_SEGMENT)), rev: optionalCount, item: optionalCount },
  "vz:text-begin": { node },
  "vz:text-cancel": { node },
  "vz:text-commit": { node, html: text, text, rev: optionalCount, item: optionalCount },
  "vz:scroll": { y: finite },
  "vz:console": { level: oneOf("log", "info", "warn", "error", "debug"), message: text },
  "vz:error": { message: text, source: text, line },
  "vz:comment-pin": { node, rect, point },
  "vz:captured": (value) => capturedDocument(value) || capturedImage(value),
  "vz:capture-failed": { id: captureId, reason: oneOf(...CAPTURE_FAILURES) },
};

const EDITOR_SHAPES = {
  "vz:highlight": { nodes: listOf(exact({ node, kind: oneOf("selection", "hover", "elo"), item: optional(optionalCount) })) },
  "vz:select-node": { node, item: optional(optionalCount) },
  "vz:begin-text-edit": { node },
  "vz:scroll-to": { node },
  "vz:mode": { mode: oneOf("select", "comment", "preview"), locale: optional(oneOf(...LOCALES)) },
  "vz:capture": { id: captureId, kind: oneOf("document", "image"), focus: optional(node), height: optional(captureHeight) },
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

function clip(value) {
  return String(value).slice(0, MAX_TEXT);
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
    if (node.nodeType !== 1 || node === skip) return;
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
  const settled = async () => {
    if (doc.fonts && doc.fonts.ready) await Promise.race([doc.fonts.ready.catch(() => null), sleep(FONT_WAIT_MS)]);
    await frames(2);
  };
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
  const canvas = await domToCanvas(doc.documentElement, { width, height: capturedHeight, scale, backgroundColor: "#ffffff", filter: (candidate) => candidate !== host });
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
.box{position:fixed;pointer-events:none;box-sizing:border-box;border-radius:2px;z-index:2147483646}
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
  shadow.innerHTML = '<div class="boxes"></div><div class="bar" role="toolbar"><button type="button" data-cmd="bold"><b>B</b></button><button type="button" data-cmd="italic"><i>I</i></button><button type="button" data-cmd="link"></button><input type="url" placeholder="https://"></div>';
  const boxes = shadow.querySelector(".boxes");
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

  const nodeOf = (target) => {
    const element = target && target.closest ? target.closest("[data-vz-node]") : null;
    return element && NODE.test(element.getAttribute("data-vz-node")) ? element : null;
  };
  const instances = (nodeRef) => {
    const escape = win.CSS && win.CSS.escape ? win.CSS.escape : (value) => value.replace(/["\\]/g, "\\$&");
    return Array.from(doc.querySelectorAll('[data-vz-node="' + escape(nodeRef) + '"]')).slice(0, MAX_ITEMS);
  };
  const byNode = (nodeRef, item) => {
    const all = instances(nodeRef);
    const wantsItem = Number.isInteger(item);
    if (picked && picked.isConnected && picked.getAttribute("data-vz-node") === nodeRef && (!wantsItem || itemOf(picked) === item)) return picked;
    return (wantsItem ? all.find((element) => itemOf(element) === item) : null) || all[0] || null;
  };

  const drawBox = (element, kind) => {
    const box = element.getBoundingClientRect();
    const div = doc.createElement("div");
    div.className = "box " + kind;
    div.style.cssText = "left:" + box.left + "px;top:" + box.top + "px;width:" + box.width + "px;height:" + box.height + "px";
    boxes.appendChild(div);
  };

  const paint = () => {
    frame = 0;
    boxes.textContent = "";
    for (const item of highlights) {
      const element = byNode(item.node, item.item);
      if (!element) continue;
      if (item.kind === "selection") {
        for (const other of instances(item.node)) if (other !== element) drawBox(other, "repeat");
      }
      drawBox(element, item.kind);
    }
    if (hovered && hovered.isConnected && mode === "select") drawBox(hovered, "hover");
    if (editing) {
      const box = editing.element.getBoundingClientRect();
      bar.style.left = Math.max(4, box.left) + "px";
      bar.style.top = Math.max(4, box.top - 36) + "px";
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
    if (!element || editing || !isInlineOnly(element)) return;
    const nodeRef = element.getAttribute("data-vz-node");
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

  doc.addEventListener("pointermove", (event) => {
    if (mode !== "select" || editing || fromOverlay(event)) return;
    const element = nodeOf(event.target);
    if (element === hovered) return;
    hovered = element;
    if (element) send({ type: "vz:hover", node: element.getAttribute("data-vz-node"), rect: rectOf(element), rev: revisionOf(revs, element.getAttribute("data-vz-node")) });
    schedule();
  }, true);
  doc.addEventListener("click", (event) => {
    if (mode === "preview" || fromOverlay(event) || (editing && editing.element.contains(event.target))) return;
    event.preventDefault();
    event.stopPropagation();
    const element = nodeOf(event.target);
    if (!element) return;
    const nodeRef = element.getAttribute("data-vz-node");
    picked = element;
    if (mode === "comment") {
      send({ type: "vz:comment-pin", node: nodeRef, rect: rectOf(element), point: pinPoint(element.getBoundingClientRect(), event.clientX, event.clientY) });
      return;
    }
    send(describe(element, nodeRef, revs));
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
    schedule();
    send({ type: "vz:scroll", y: win.scrollY });
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

  win.addEventListener("message", (event) => {
    const message = acceptEditorMessage(event, editorOrigin, parent);
    if (!message) return;
    if (message.type === "vz:capture") {
      capture(message);
      return;
    }
    if (message.type === "vz:mode") {
      if (message.mode !== "select") finishEdit(true);
      mode = message.mode;
      if (message.locale) label(message.locale);
      hovered = null;
    } else if (message.type === "vz:highlight") {
      highlights = message.nodes;
    } else if (message.type === "vz:select-node") {
      const element = byNode(message.node, message.item);
      if (element) {
        picked = element;
        send(describe(element, message.node, revs));
      }
    } else if (message.type === "vz:begin-text-edit") {
      beginEdit(byNode(message.node));
    } else if (message.type === "vz:scroll-to") {
      const element = byNode(message.node);
      const reduce = win.matchMedia && win.matchMedia("(prefers-reduced-motion: reduce)").matches;
      if (element) element.scrollIntoView({ block: "center", behavior: reduce ? "auto" : "smooth" });
    }
    schedule();
  });

  send({ type: "vz:ready" });
}

if (typeof window !== "undefined" && window.document && window.parent && window.parent !== window) startBridge(window, window.parent);

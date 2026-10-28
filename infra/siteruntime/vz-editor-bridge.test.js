import { expect, test } from "bun:test";

import fixture from "./testdata/bridge_messages.json";
import { acceptEditorMessage, decodeEditorMessage, decodeFrameMessage, editorOriginFrom, elementPath, isInlineOnly, itemIndex, pinPoint, revisionOf, revisionsFrom, sanitizeInline } from "./vz-editor-bridge.js";

function text(value) {
  return { nodeType: 3, nodeName: "#text", nodeValue: value, childNodes: [] };
}

function el(tag, attrs, ...children) {
  const node = { nodeType: 1, nodeName: tag.toUpperCase(), tagName: tag.toUpperCase(), childNodes: children, children: [], parentElement: null };
  node.getAttribute = (name) => (attrs && Object.hasOwn(attrs, name) ? attrs[name] : null);
  for (const child of children) {
    if (child.nodeType === 1) {
      child.parentElement = node;
      node.children.push(child);
    }
  }
  return node;
}

test("decodes every frame message of the shared fixture and refuses the bad ones", () => {
  for (const message of fixture.fromFrame.valid) expect(decodeFrameMessage(message)).toEqual(message);
  for (const sample of fixture.fromFrame.invalid) expect(decodeFrameMessage(sample.message), sample.why).toBeNull();
});

test("decodes every editor message of the shared fixture and refuses the bad ones", () => {
  for (const message of fixture.fromEditor.valid) expect(decodeEditorMessage(message)).toEqual(message);
  for (const sample of fixture.fromEditor.invalid) expect(decodeEditorMessage(sample.message), sample.why).toBeNull();
});

test("refuses text longer than the protocol allows", () => {
  expect(decodeFrameMessage({ type: "vz:console", level: "log", message: "x".repeat(20001) })).toBeNull();
  expect(decodeFrameMessage({ type: "vz:console", level: "log", message: "x".repeat(20000) })).not.toBeNull();
});

test("reads the editor origin from the fragment and checks it against the real parent", () => {
  const hash = "#vz-editor=https%3A%2F%2Fapp.vozkoia.com";
  expect(editorOriginFrom(hash, undefined)).toBe("https://app.vozkoia.com");
  expect(editorOriginFrom(hash, ["https://app.vozkoia.com"])).toBe("https://app.vozkoia.com");
  expect(editorOriginFrom(hash, ["https://evil.example"])).toBeNull();
  expect(editorOriginFrom(hash, [])).toBeNull();
  expect(editorOriginFrom("#vz-editor=https%3A%2F%2Fapp.vozkoia.com%2F", undefined)).toBeNull();
  expect(editorOriginFrom("#vz-editor=javascript%3Aalert(1)", undefined)).toBeNull();
  expect(editorOriginFrom("#topo", undefined)).toBeNull();
  expect(editorOriginFrom("", undefined)).toBeNull();
});

test("accepts an editor message only from the exact origin and the parent window", () => {
  const parent = {};
  const origin = "https://app.vozkoia.com";
  const data = { type: "vz:mode", mode: "select" };
  expect(acceptEditorMessage({ origin, source: parent, data }, origin, parent)).toEqual(data);
  expect(acceptEditorMessage({ origin: "https://app.vozkoia.com.evil.com", source: parent, data }, origin, parent)).toBeNull();
  expect(acceptEditorMessage({ origin, source: {}, data }, origin, parent)).toBeNull();
  expect(acceptEditorMessage({ origin, source: parent, data: { type: "vz:ready" } }, origin, parent)).toBeNull();
  expect(acceptEditorMessage({ origin, source: parent, data }, null, parent)).toBeNull();
});

test("keeps only bold, italic and safe links from an inline edit", () => {
  const edited = el(
    "h1",
    {},
    text("Agende "),
    el("b", {}, text("hoje")),
    text(" & "),
    el("span", { style: "color:red" }, el("em", {}, text("ganhe"))),
    text(" "),
    el("a", { href: " /obrigado ", class: "x", target: "_blank", onclick: "steal()" }, text("avaliação")),
    el("br", {}),
    el("a", { href: "javascript:alert(1)" }, text("clique")),
    el("script", {}, text("<x>")),
    { nodeType: 8, nodeName: "#comment", nodeValue: "nota", childNodes: [] },
  );
  expect(sanitizeInline(edited)).toEqual({
    html: 'Agende <b>hoje</b> &amp; <em>ganhe</em> <a href="/obrigado" target="_blank" rel="noopener">avaliação</a> clique',
    text: "Agende hoje & ganhe avaliação clique",
  });
});

test("escapes quotes in link targets and keeps mail and phone links", () => {
  const edited = el("p", {}, el("a", { href: 'https://a.example/?q="x"' }, text("a")), el("a", { href: "mailto:oi@a.example" }, text("b")), el("a", { href: "tel:+5511" }, text("c")), el("a", { href: "//evil.example" }, text("d")));
  expect(sanitizeInline(edited).html).toBe('<a href="https://a.example/?q=&quot;x&quot;">a</a><a href="mailto:oi@a.example">b</a><a href="tel:+5511">c</a>d');
});

test("edits in place only elements whose content is inline", () => {
  expect(isInlineOnly(el("h1", {}, text("a"), el("strong", {}, text("b"))))).toBe(true);
  expect(isInlineOnly(el("section", {}, el("div", {}, text("a"))))).toBe(false);
  expect(isInlineOnly(el("p", {}, el("img", {})))).toBe(false);
  expect(isInlineOnly(el("p", {}))).toBe(true);
});

test("describes where an element sits below the body", () => {
  const h1 = el("h1", {}, text("x"));
  const section = el("section", {}, el("p", {}), h1);
  const main = el("main", {}, el("section", {}), section);
  const body = el("body", {}, el("header", {}), main);
  el("html", {}, body);
  expect(elementPath(h1)).toEqual(["main:1", "section:1", "h1:1"]);
  expect(elementPath(body)).toEqual([]);
});

test("pins a comment as a fraction of the element box", () => {
  expect(pinPoint({ left: 100, top: 50, width: 200, height: 100 }, 150, 100)).toEqual({ x: 0.25, y: 0.5 });
  expect(pinPoint({ left: 100, top: 50, width: 200, height: 100 }, 400, 0)).toEqual({ x: 1, y: 0 });
  expect(pinPoint({ left: 0, top: 0, width: 0, height: 0 }, 10, 10)).toEqual({ x: 0, y: 0 });
});

test("reads the source revision of each file from the build's meta tag", () => {
  const revs = revisionsFrom('{"src/routes/index.jsx":7,"pages/index.html":0,"bad.html":-1,"../x.html":2,"half.html":1.5}');
  expect(revs).toEqual({ "src/routes/index.jsx": 7, "pages/index.html": 0 });
  expect(revisionOf(revs, "src/routes/index.jsx#120")).toBe(7);
  expect(revisionOf(revs, "pages/index.html#0")).toBe(0);
  expect(revisionOf(revs, "pages/obrigado.html#5")).toBeNull();
  expect(revisionsFrom("not json")).toEqual({});
  expect(revisionsFrom(null)).toEqual({});
  expect(revisionsFrom("[1,2]")).toEqual({});
});

test("reads the index of an element rendered from a literal list", () => {
  expect(itemIndex("0")).toBe(0);
  expect(itemIndex("12")).toBe(12);
  expect(itemIndex(null)).toBeNull();
  expect(itemIndex("-1")).toBeNull();
  expect(itemIndex("1.5")).toBeNull();
  expect(itemIndex("01")).toBeNull();
  expect(itemIndex("x")).toBeNull();
});

test("refuses capture replies larger than the protocol allows", () => {
  const html = (length) => ({ type: "vz:captured", id: "d", kind: "document", html: "x".repeat(length) });
  expect(decodeFrameMessage(html(1_572_864))).not.toBeNull();
  expect(decodeFrameMessage(html(1_572_865))).toBeNull();
});

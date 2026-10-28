import { afterEach, expect, test } from "bun:test";
import { Window } from "happy-dom";

import { startBridge } from "./vz-editor-bridge.js";

const EDITOR = "https://app.vozkoia.com";
const PREVIEW = "https://r1.preview.vozko.site/";
const BODY = `<main>
<h1 data-vz-node="pages/index.html#10">Sorriso <b>novo</b></h1>
<ul><li data-vz-i="2"><p data-vz-node="src/App.jsx#40">Item</p></li></ul>
<section data-vz-node="pages/index.html#90"><div>bloco</div></section>
</main>`;
const opened = [];

afterEach(async () => {
  while (opened.length) await opened.pop().happyDOM.abort();
});

function setup({ hash = "#vz-editor=" + encodeURIComponent(EDITOR) } = {}) {
  const window = new Window({ url: PREVIEW + "obrigado" + hash });
  opened.push(window);
  const doc = window.document;
  doc.head.innerHTML = `<meta name="vz-revs" content='{"pages/index.html":4}'>`;
  doc.body.innerHTML = BODY;
  const commands = [];
  doc.execCommand = (command, ui, value) => commands.push(value === undefined ? [command] : [command, value]) > 0;
  const sent = [];
  const parent = { postMessage: (message, origin) => sent.push({ message, origin }) };
  startBridge(window, parent);
  const fromEditor = (data, origin = EDITOR, source = parent) => window.dispatchEvent(new window.MessageEvent("message", { data, origin, source }));
  const messages = (type) => sent.map((s) => s.message).filter((m) => m.type === type);
  const toolbar = () => doc.querySelector("vz-bridge-overlay").shadowRoot;
  return { window, doc, sent, parent, commands, fromEditor, messages, toolbar };
}

function click(page, element, init = {}) {
  const event = new page.window.MouseEvent("click", { bubbles: true, cancelable: true, ...init });
  element.dispatchEvent(event);
  return event;
}

function key(page, element, init) {
  const event = new page.window.KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  element.dispatchEvent(event);
  return event;
}

const h1 = (page) => page.doc.querySelector("h1");

function editing(page) {
  page.fromEditor({ type: "vz:mode", mode: "select" });
  page.fromEditor({ type: "vz:begin-text-edit", node: "pages/index.html#10" });
  return h1(page);
}

test("announces itself only to the exact editor origin and hides the fragment", () => {
  const page = setup();
  expect(page.sent).toEqual([{ message: { type: "vz:ready" }, origin: EDITOR }]);
  expect(page.window.location.hash).toBe("");
  expect(page.window.sessionStorage.getItem("vz-editor-origin")).toBe(EDITOR);
});

test("stays silent and inert without a valid editor origin", () => {
  for (const hash of ["", "#vz-editor=javascript%3Aalert(1)", "#vz-editor=" + encodeURIComponent(EDITOR + "/")]) {
    const page = setup({ hash });
    page.fromEditor({ type: "vz:mode", mode: "select" });
    expect(click(page, h1(page)).defaultPrevented).toBe(false);
    expect(page.sent).toEqual([]);
  }
});

test("ignores editor messages from another origin, another window or of unknown shape", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" }, "https://evil.example");
  page.fromEditor({ type: "vz:mode", mode: "select" }, EDITOR, {});
  page.fromEditor({ type: "vz:mode", mode: "select", extra: 1 });
  page.fromEditor({ type: "vz:explode" });
  expect(click(page, h1(page)).defaultPrevented).toBe(false);
  page.fromEditor({ type: "vz:mode", mode: "select" });
  expect(click(page, h1(page)).defaultPrevented).toBe(true);
});

test("reports selection with the file revision and the list item index", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  click(page, h1(page).querySelector("b"));
  click(page, page.doc.querySelector("p"));
  const [heading, item] = page.messages("vz:select");
  expect(heading).toMatchObject({ node: "pages/index.html#10", tag: "h1", text: "Sorriso novo", rev: 4, item: null, path: ["main:0", "h1:0"], attrs: { "data-vz-node": "pages/index.html#10" } });
  expect(item).toMatchObject({ node: "src/App.jsx#40", tag: "p", rev: null, item: 2, path: ["main:0", "ul:1", "li:0", "p:0"] });
  expect(page.sent.every((s) => s.origin === EDITOR)).toBe(true);
});

test("reports hover and comment pins", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  h1(page).dispatchEvent(new page.window.PointerEvent("pointermove", { bubbles: true }));
  expect(page.messages("vz:hover")).toEqual([{ type: "vz:hover", node: "pages/index.html#10", rect: expect.any(Object), rev: 4 }]);
  page.fromEditor({ type: "vz:mode", mode: "comment" });
  click(page, page.doc.querySelector("section"));
  expect(page.messages("vz:comment-pin")).toEqual([{ type: "vz:comment-pin", node: "pages/index.html#90", rect: expect.any(Object), point: { x: 0, y: 0 } }]);
});

test("edits text in place and commits the inline markup and plain text on Enter", () => {
  const page = setup();
  const heading = editing(page);
  expect(heading.getAttribute("contenteditable")).toBe("true");
  expect(page.messages("vz:text-begin")).toEqual([{ type: "vz:text-begin", node: "pages/index.html#10" }]);
  heading.innerHTML = 'Sorriso <b>novo</b> já <span style="color:red">hoje</span>';
  expect(key(page, heading, { key: "Enter" }).defaultPrevented).toBe(true);
  expect(page.messages("vz:text-commit")).toEqual([{ type: "vz:text-commit", node: "pages/index.html#10", html: "Sorriso <b>novo</b> já hoje", text: "Sorriso novo já hoje", rev: 4, item: null }]);
  expect(heading.hasAttribute("contenteditable")).toBe(false);
});

test("cancels on Escape and restores what was there", () => {
  const page = setup();
  const heading = editing(page);
  heading.innerHTML = "apagado";
  key(page, heading, { key: "Escape" });
  expect(heading.innerHTML).toBe("Sorriso <b>novo</b>");
  expect(page.messages("vz:text-cancel")).toEqual([{ type: "vz:text-cancel", node: "pages/index.html#10" }]);
  expect(page.messages("vz:text-commit")).toEqual([]);
});

test("an edit that changed nothing is a cancel, not a commit", () => {
  const page = setup();
  key(page, editing(page), { key: "Enter" });
  expect(page.messages("vz:text-commit")).toEqual([]);
  expect(page.messages("vz:text-cancel")).toHaveLength(1);
});

test("starts editing on double click only where the content is inline", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  page.doc.querySelector("section").dispatchEvent(new page.window.MouseEvent("dblclick", { bubbles: true, cancelable: true }));
  expect(page.doc.querySelector("section").hasAttribute("contenteditable")).toBe(false);
  h1(page).dispatchEvent(new page.window.MouseEvent("dblclick", { bubbles: true, cancelable: true }));
  expect(h1(page).getAttribute("contenteditable")).toBe("true");
});

test("offers bold, italic and link and nothing else", () => {
  const page = setup();
  const heading = editing(page);
  const buttons = page.toolbar().querySelectorAll("button");
  expect(Array.from(buttons).map((b) => b.getAttribute("data-cmd"))).toEqual(["bold", "italic", "link"]);
  click(page, buttons[0]);
  click(page, buttons[1]);
  key(page, heading, { key: "b", ctrlKey: true });
  key(page, heading, { key: "i", metaKey: true });
  expect(page.commands).toEqual([["bold"], ["italic"], ["bold"], ["italic"]]);

  click(page, buttons[2]);
  const input = page.toolbar().querySelector("input");
  expect(page.toolbar().querySelector(".bar").className).toContain("link");
  input.value = "https://clinica.example/agenda";
  key(page, input, { key: "Enter" });
  click(page, buttons[2]);
  input.value = "javascript:alert(1)";
  key(page, input, { key: "Enter" });
  expect(page.commands.slice(4)).toEqual([["createLink", "https://clinica.example/agenda"], ["unlink"]]);
  expect(heading.getAttribute("contenteditable")).toBe("true");
});

test("pastes only plain text on one line", () => {
  const page = setup();
  const heading = editing(page);
  const data = new page.window.DataTransfer();
  data.setData("text/html", "<img src=x onerror=alert(1)>");
  data.setData("text/plain", "linha um\n\n   linha dois");
  const event = new page.window.ClipboardEvent("paste", { bubbles: true, cancelable: true, clipboardData: data });
  heading.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(true);
  expect(page.commands).toEqual([["insertText", "linha um linha dois"]]);
});

test("labels the toolbar in the editor's language, Portuguese by default", () => {
  const page = setup();
  editing(page);
  const label = (index) => page.toolbar().querySelectorAll("button")[index];
  expect([label(0).getAttribute("aria-label"), label(1).getAttribute("title"), label(2).textContent]).toEqual(["Negrito", "Itálico", "Link"]);
  page.fromEditor({ type: "vz:mode", mode: "select", locale: "es" });
  expect([label(0).getAttribute("aria-label"), label(1).getAttribute("title"), label(2).textContent]).toEqual(["Negrita", "Cursiva", "Enlace"]);
  expect(page.toolbar().querySelector("input").getAttribute("aria-label")).toBe("Dirección del enlace");
  page.fromEditor({ type: "vz:mode", mode: "select", locale: "de" });
  expect(label(0).getAttribute("title")).toBe("Fett");
  page.fromEditor({ type: "vz:mode", mode: "select", locale: "en" });
  expect(label(1).getAttribute("aria-label")).toBe("Italic");
});

test("commits when focus leaves the element for the page, not for the toolbar", () => {
  const page = setup();
  const heading = editing(page);
  heading.innerHTML = "Sorriso novo hoje";
  heading.dispatchEvent(new page.window.FocusEvent("focusout", { bubbles: true, relatedTarget: page.toolbar().querySelector("input") }));
  expect(page.messages("vz:text-commit")).toEqual([]);
  heading.dispatchEvent(new page.window.FocusEvent("focusout", { bubbles: true, relatedTarget: page.doc.querySelector("section") }));
  expect(page.messages("vz:text-commit")).toEqual([{ type: "vz:text-commit", node: "pages/index.html#10", html: "Sorriso novo hoje", text: "Sorriso novo hoje", rev: 4, item: null }]);
});

const REPEATED = `<main><ul>
<li data-vz-i="0"><p data-vz-node="src/App.jsx#40">Clínica geral</p></li>
<li data-vz-i="1"><p data-vz-node="src/App.jsx#40">Endodontia</p></li>
</ul></main>`;

test("a repeated element keeps the instance the person picked, by item after a reload", async () => {
  const page = setup();
  page.doc.body.innerHTML = REPEATED;
  page.fromEditor({ type: "vz:mode", mode: "select" });
  page.fromEditor({ type: "vz:select-node", node: "src/App.jsx#40", item: 1 });
  expect(page.messages("vz:select").at(-1)).toMatchObject({ node: "src/App.jsx#40", text: "Endodontia", item: 1 });
  page.fromEditor({ type: "vz:select-node", node: "src/App.jsx#40" });
  expect(page.messages("vz:select").at(-1)).toMatchObject({ text: "Endodontia", item: 1 });
  click(page, page.doc.querySelectorAll("p")[0]);
  page.fromEditor({ type: "vz:select-node", node: "src/App.jsx#40" });
  expect(page.messages("vz:select").at(-1)).toMatchObject({ text: "Clínica geral", item: 0 });
});

test("the selection outlines the picked instance and marks its repeats", async () => {
  const page = setup();
  page.doc.body.innerHTML = REPEATED;
  page.fromEditor({ type: "vz:mode", mode: "select" });
  click(page, page.doc.querySelectorAll("p")[1]);
  page.fromEditor({ type: "vz:highlight", nodes: [{ node: "src/App.jsx#40", kind: "selection", item: 1 }] });
  await new Promise((resolve) => page.window.requestAnimationFrame(resolve));
  await new Promise((resolve) => page.window.requestAnimationFrame(resolve));
  const boxes = page.toolbar().querySelector(".boxes");
  expect(boxes.querySelectorAll(".box.selection").length).toBe(1);
  expect(boxes.querySelectorAll(".box.repeat").length).toBe(1);
});

function captureSetup(screenshot) {
  const window = new Window({ url: PREVIEW + "#vz-editor=" + encodeURIComponent(EDITOR) });
  opened.push(window);
  const doc = window.document;
  doc.head.innerHTML = `<meta name="vz-revs" content='{"pages/index.html":4}'><style>p > b { color: red }</style>`;
  doc.body.innerHTML = BODY;
  const sent = [];
  const parent = { postMessage: (message) => sent.push(message) };
  startBridge(window, parent, { loadScreenshot: async () => screenshot });
  const fromEditor = (data, origin = EDITOR) => window.dispatchEvent(new window.MessageEvent("message", { data, origin, source: parent }));
  const reply = async (id) => {
    for (let i = 0; i < 400; i++) {
      const found = sent.find((m) => (m.type === "vz:captured" || m.type === "vz:capture-failed") && m.id === id);
      if (found) return found;
      await new Promise((resolve) => setTimeout(resolve, 5));
    }
    return null;
  };
  return { window, doc, sent, fromEditor, reply };
}

function fakeScreenshot(dataUrl = "data:image/jpeg;base64,/9j/AAAA") {
  const calls = [];
  return {
    calls,
    domToCanvas: async (node, options) => {
      calls.push({ node, options });
      return { toDataURL: (type, quality) => (type === "image/jpeg" && quality === 0.85 ? dataUrl : "") };
    },
  };
}

test("serializes the live document for the editor and leaves its own overlay out", async () => {
  const page = captureSetup(fakeScreenshot());
  page.doc.body.innerHTML = `<p title="a &amp; &quot;b&quot;">1 &lt; 2 &amp; 3<!-- nota --><br><img src="x.webp"></p><script>if (a < b && c) {}</script><template><i>t</i></template><div id="host"></div>`;
  page.doc.getElementById("host").attachShadow({ mode: "open" }).innerHTML = "<span>sombra</span>";
  page.fromEditor({ type: "vz:capture", id: "d1", kind: "document" }, "https://evil.example");
  page.fromEditor({ type: "vz:capture", id: "d1", kind: "document" });
  const answer = await page.reply("d1");
  expect(answer.kind).toBe("document");
  expect(answer.html.startsWith("<!doctype html><html>")).toBe(true);
  expect(answer.html).toContain(`<p title="a &amp; &quot;b&quot;">1 &lt; 2 &amp; 3<br><img src="x.webp"></p>`);
  expect(answer.html).toContain("<script>if (a < b && c) {}</script>");
  expect(answer.html).toContain("<template><i>t</i></template>");
  expect(answer.html).toContain(`<div id="host"><template shadowrootmode="open"><span>sombra</span></template></div>`);
  expect(answer.html).toContain("<style>p > b { color: red }</style>");
  expect(answer.html).not.toContain("vz-bridge-overlay");
  expect(answer.html).not.toContain("nota");
  expect(page.sent.filter((m) => m.type === "vz:captured")).toHaveLength(1);
});

test("refuses a document larger than the protocol carries", async () => {
  const page = captureSetup(fakeScreenshot());
  page.doc.body.textContent = "x".repeat(1_572_864);
  page.fromEditor({ type: "vz:capture", id: "big", kind: "document" });
  expect(await page.reply("big")).toEqual({ type: "vz:capture-failed", id: "big", reason: "too_large" });
});

test("renders the page to a jpeg with the facts the backend checks", async () => {
  const shot = fakeScreenshot();
  const page = captureSetup(shot);
  page.fromEditor({ type: "vz:capture", id: "i1", kind: "image" });
  const answer = await page.reply("i1");
  expect(answer).toMatchObject({ type: "vz:captured", id: "i1", kind: "image", image: "data:image/jpeg;base64,/9j/AAAA", truncated: false, focus: null, boxes: [] });
  expect(answer.scale).toBeGreaterThan(0);
  expect(answer.scale).toBeLessThanOrEqual(1);
  expect(Object.keys(answer.facts).sort()).toEqual(["consoleErrors", "failedRequests", "images", "metrics", "overflow", "scrollWidth", "texts", "transferBytes"]);
  expect(shot.calls).toHaveLength(1);
  expect(shot.calls[0].node).toBe(page.doc.documentElement);
  expect(shot.calls[0].options.backgroundColor).toBe("#ffffff");
  expect(shot.calls[0].options.filter(page.doc.querySelector("vz-bridge-overlay"))).toBe(false);
  expect(shot.calls[0].options.filter(page.doc.querySelector("h1"))).toBe(true);
});

test("a fixed height renders only the top of the page at full scale", async () => {
  const shot = fakeScreenshot();
  const page = captureSetup(shot);
  page.fromEditor({ type: "vz:capture", id: "t1", kind: "image", height: 900 });
  const answer = await page.reply("t1");
  expect(answer).toMatchObject({ kind: "image", capturedHeight: 900, scale: 1, truncated: false });
  expect(shot.calls[0].options).toMatchObject({ height: 900, scale: 1 });
});

test("says when the focused element is not on the page", async () => {
  const page = captureSetup(fakeScreenshot());
  page.fromEditor({ type: "vz:capture", id: "f1", kind: "image", focus: "pages/index.html#999" });
  expect(await page.reply("f1")).toEqual({ type: "vz:capture-failed", id: "f1", reason: "focus_missing" });
});

test("reports a failed render and an oversized image without throwing", async () => {
  const broken = captureSetup({ domToCanvas: async () => { throw new Error("tainted"); } });
  broken.fromEditor({ type: "vz:capture", id: "x1", kind: "image" });
  expect(await broken.reply("x1")).toEqual({ type: "vz:capture-failed", id: "x1", reason: "capture_failed" });
  const huge = captureSetup(fakeScreenshot("data:image/jpeg;base64," + "A".repeat(16_000_000)));
  huge.fromEditor({ type: "vz:capture", id: "x2", kind: "image" });
  expect(await huge.reply("x2")).toEqual({ type: "vz:capture-failed", id: "x2", reason: "too_large" });
});

test("hands console errors seen since the start to the facts", async () => {
  const page = captureSetup(fakeScreenshot());
  page.window.console.error("falhou", { a: 1 });
  page.window.console.log("só um log");
  page.fromEditor({ type: "vz:capture", id: "c1", kind: "image" });
  const answer = await page.reply("c1");
  expect(answer.facts.consoleErrors).toEqual(['falhou {"a":1}']);
});

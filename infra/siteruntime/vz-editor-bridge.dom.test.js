import { afterEach, expect, test } from "bun:test";
import { Window } from "happy-dom";

import { elementPath, guardHover, guardHoverSheets, startBridge } from "./vz-editor-bridge.js";

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

async function announced(page, type = "vz:ready") {
  for (let i = 0; i < 200 && page.messages(type).length === 0; i++) await new Promise((resolve) => setTimeout(resolve, 5));
}

test("announces itself once the page has painted, only to the exact editor origin, and hides the fragment", async () => {
  const page = setup();
  expect(page.sent).toEqual([]);
  await announced(page);
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

test("restores the scroll position the editor hands a new frame and confirms where it landed", async () => {
  const page = setup();
  page.fromEditor({ type: "vz:scroll-y", y: 320 });
  await announced(page, "vz:scroll");
  expect(page.window.scrollY).toBe(320);
  expect(page.messages("vz:scroll").at(-1)).toEqual({ type: "vz:scroll", y: 320 });
});

test("reports the parent content box and the shown children with the selection and after each live change", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  const section = page.doc.querySelector("section");
  const hidden = page.doc.createElement("div");
  hidden.style.display = "none";
  section.appendChild(hidden);
  click(page, section);
  const [selected] = page.messages("vz:select");
  const box = { x: expect.any(Number), y: expect.any(Number), width: expect.any(Number), height: expect.any(Number) };
  expect(selected.parent).toEqual(box);
  expect(selected.children).toEqual([box]);
  applyChange(page, "e1", "pages/index.html#90", { kind: "classes", add: ["gap-4"], remove: [] });
  expect(page.messages("vz:styles").at(-1)).toMatchObject({ node: "pages/index.html#90", rect: box, parent: box, children: [box] });
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

test("hands Ctrl or Cmd+S to the editor instead of the browser save dialog, finishing a text edit first", () => {
  const page = setup();
  const pressed = key(page, h1(page), { key: "s", ctrlKey: true });
  expect(pressed.defaultPrevented).toBe(true);
  expect(page.messages("vz:save")).toEqual([{ type: "vz:save" }]);
  const element = editing(page);
  const mac = key(page, element, { key: "S", metaKey: true, shiftKey: true });
  expect(mac.defaultPrevented).toBe(false);
  key(page, element, { key: "s", metaKey: true });
  expect(page.messages("vz:save")).toHaveLength(2);
  expect(page.messages("vz:text-cancel")).toHaveLength(1);
  expect(page.sent.at(-1).message).toEqual({ type: "vz:save" });
  expect(key(page, element, { key: "s" }).defaultPrevented).toBe(false);
});

function applyChange(page, id, node, change, item) {
  page.fromEditor({ type: "vz:apply", id, node, change, ...(item === undefined ? {} : { item }) });
}

test("tells the editor the computed style and the root size of the selection", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  click(page, h1(page));
  const [selected] = page.messages("vz:select");
  expect(typeof selected.computed).toBe("object");
  expect(Object.keys(selected.computed)).toContain("display");
  expect(selected.rem).toBeGreaterThan(0);
});

test("applies a live class change to the known node at once, replaces it by id and reverts it", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  const heading = h1(page);
  heading.className = "py-4 text-xl";
  click(page, heading);
  applyChange(page, "e1", "pages/index.html#10", { kind: "classes", add: ["md:py-8"], remove: ["text-xl"] });
  expect(Array.from(heading.classList)).toEqual(["py-4", "md:py-8"]);
  expect(page.messages("vz:applied")).toEqual([{ type: "vz:applied", id: "e1" }]);
  expect(page.messages("vz:styles").at(-1)).toMatchObject({ node: "pages/index.html#10", classes: ["py-4", "md:py-8"] });
  applyChange(page, "e1", "pages/index.html#10", { kind: "classes", add: ["md:py-12"], remove: ["text-xl"] });
  expect(Array.from(heading.classList)).toEqual(["py-4", "md:py-12"]);
  page.fromEditor({ type: "vz:revert", id: "e1" });
  expect(Array.from(heading.classList)).toEqual(["py-4", "text-xl"]);
});

test("applies attributes, removal, duplication and moves live, each reverted by its id", () => {
  const page = setup();
  const section = page.doc.querySelector("section");
  applyChange(page, "a1", "pages/index.html#90", { kind: "attr", name: "id", value: "planos" });
  expect(section.id).toBe("planos");
  applyChange(page, "r1", "pages/index.html#90", { kind: "remove" });
  expect(section.style.display).toBe("none");
  page.fromEditor({ type: "vz:revert", id: "r1" });
  expect(section.style.display).toBe("");
  applyChange(page, "d1", "pages/index.html#90", { kind: "duplicate" });
  expect(page.doc.querySelectorAll("section").length).toBe(2);
  page.fromEditor({ type: "vz:revert", id: "d1" });
  expect(page.doc.querySelectorAll("section").length).toBe(1);
  applyChange(page, "m1", "pages/index.html#90", { kind: "move", anchor: "pages/index.html#10", place: "before" });
  expect(page.doc.querySelector("main").firstElementChild).toBe(section);
  page.fromEditor({ type: "vz:revert", id: "m1" });
  expect(page.doc.querySelector("main").lastElementChild).toBe(section);
  page.fromEditor({ type: "vz:revert", id: "a1" });
  expect(section.hasAttribute("id")).toBe(false);
});

test("refuses unsafe or unknown live changes whole, reports them and changes nothing", () => {
  const page = setup();
  const heading = h1(page);
  heading.className = "py-4";
  const cases = [
    ["n1", "pages/index.html#999", { kind: "classes", add: ["py-8"], remove: [] }, "node_unknown"],
    ["c1", "pages/index.html#10", { kind: "classes", add: ["py-8", "a;b"], remove: ["py-4"] }, "class_invalid"],
    ["h1", "pages/index.html#10", { kind: "attr", name: "href", value: "javascript:alert(1)" }, "attribute_forbidden"],
    ["h2", "pages/index.html#10", { kind: "attr", name: "href", value: "data:text/html,x" }, "attribute_forbidden"],
    ["o1", "pages/index.html#10", { kind: "attr", name: "onclick", value: "x()" }, "attribute_forbidden"],
    ["s1", "pages/index.html#10", { kind: "attr", name: "style", value: "color:red" }, "attribute_forbidden"],
    ["s2", "pages/index.html#10", { kind: "attr", name: "srcdoc", value: "<script>x</script>" }, "attribute_forbidden"],
    ["m1", "pages/index.html#10", { kind: "move", anchor: "pages/index.html#999", place: "after" }, "node_unknown"],
  ];
  for (const [id, node, change, reason] of cases) {
    applyChange(page, id, node, change);
    expect(page.messages("vz:apply-refused").at(-1)).toEqual({ type: "vz:apply-refused", id, reason });
  }
  expect(Array.from(heading.classList)).toEqual(["py-4"]);
  expect(page.messages("vz:applied")).toEqual([]);
  expect(heading.hasAttribute("href") || heading.hasAttribute("onclick") || heading.hasAttribute("style") || heading.hasAttribute("srcdoc")).toBe(false);
  expect(page.doc.querySelector("main").firstElementChild).toBe(heading);
});

test("never duplicates an element that holds a frame or a script", () => {
  const page = setup();
  const section = page.doc.querySelector("section");
  section.appendChild(page.doc.createElement("iframe"));
  applyChange(page, "d1", "pages/index.html#90", { kind: "duplicate" });
  expect(page.messages("vz:apply-refused").at(-1)).toEqual({ type: "vz:apply-refused", id: "d1", reason: "element_forbidden" });
  expect(page.doc.querySelectorAll("section").length).toBe(1);
});

test("ignores live changes from any origin or window other than the editor", () => {
  const page = setup();
  const section = page.doc.querySelector("section");
  page.fromEditor({ type: "vz:apply", id: "x1", node: "pages/index.html#90", change: { kind: "attr", name: "id", value: "a" } }, "https://evil.example");
  page.fromEditor({ type: "vz:apply", id: "x2", node: "pages/index.html#90", change: { kind: "attr", name: "id", value: "b" } }, EDITOR, { postMessage() {} });
  page.fromEditor({ type: "vz:apply", id: "x3", node: "pages/index.html#90", change: { kind: "html", html: "<img onerror=x>" } });
  expect(section.hasAttribute("id")).toBe(false);
  expect(page.messages("vz:apply-refused")).toEqual([]);
  expect(page.messages("vz:applied")).toEqual([]);
});

const PLATFORM_BODY = `<main>
<section data-vz-node="pages/index.html#200" style="color: rgb(1, 2, 3); font-size: 20px">
<div style="text-align: center"><p data-vz-node="pages/index.html#300" style="font-weight: 700">Texto</p></div>
<vz-booking data-vz-node="pages/index.html#220" booking="agenda"><div data-vz-booking-root><button>10:00</button></div></vz-booking>
<vz-webchat data-vz-node="pages/index.html#260" widget="wk"><span data-vz-node="pages/index.html#270">Chat</span></vz-webchat>
</section>
</main>`;

function platformPage() {
  const page = setup();
  page.doc.body.innerHTML = PLATFORM_BODY;
  page.fromEditor({ type: "vz:mode", mode: "select" });
  return page;
}

test("a booking or site chat block is selected whole, wherever inside it the click lands", () => {
  const page = platformPage();
  click(page, page.doc.querySelector("vz-booking button"));
  click(page, page.doc.querySelector("vz-webchat span"));
  const [booking, chat] = page.messages("vz:select");
  expect(booking).toMatchObject({ node: "pages/index.html#220", tag: "vz-booking" });
  expect(chat).toMatchObject({ node: "pages/index.html#260", tag: "vz-webchat" });
  page.window.document.querySelector("vz-webchat span").dispatchEvent(new page.window.MouseEvent("pointermove", { bubbles: true }));
  expect(page.messages("vz:hover").map((m) => m.node)).toEqual(["pages/index.html#260"]);
});

test("a booking or site chat block is never text editable", () => {
  const page = platformPage();
  for (const tag of ["vz-booking", "vz-webchat"]) {
    page.doc.querySelector(tag).dispatchEvent(new page.window.MouseEvent("dblclick", { bubbles: true, cancelable: true }));
    expect(page.doc.querySelector(tag).hasAttribute("contenteditable")).toBe(false);
  }
  page.doc.querySelector("vz-webchat span").dispatchEvent(new page.window.MouseEvent("dblclick", { bubbles: true, cancelable: true }));
  page.fromEditor({ type: "vz:begin-text-edit", node: "pages/index.html#270" });
  page.fromEditor({ type: "vz:begin-text-edit", node: "pages/index.html#260" });
  expect(page.doc.querySelector("[contenteditable]")).toBeNull();
  expect(page.messages("vz:text-begin")).toEqual([]);
});

test("names the nearest element that sets each inherited text style", () => {
  const page = platformPage();
  click(page, page.doc.querySelector("p"));
  const [selected] = page.messages("vz:select");
  expect(selected.inherited).toEqual({
    color: { node: "pages/index.html#200", tag: "section" },
    "font-size": { node: "pages/index.html#200", tag: "section" },
  });
  page.fromEditor({ type: "vz:apply", id: "c1", node: "pages/index.html#300", change: { kind: "classes", add: ["mt-2"], remove: [] } });
  const styles = page.messages("vz:styles").at(-1);
  expect(styles.inherited).toEqual(selected.inherited);
});

test("leaves out styles the element sets itself or that only the page sets", () => {
  const page = platformPage();
  click(page, page.doc.querySelector("section"));
  const [selected] = page.messages("vz:select");
  expect(selected.inherited).toEqual({});
});

test("tells the editor the selection's parent and its neighbours, by node", () => {
  const page = platformPage();
  click(page, page.doc.querySelector("vz-booking button"));
  click(page, page.doc.querySelector("p"));
  click(page, page.doc.querySelector("section"));
  const [booking, paragraph, section] = page.messages("vz:select");
  expect(booking.family).toEqual({ parent: "pages/index.html#200", previous: "", next: "pages/index.html#260" });
  expect(paragraph.family).toEqual({ parent: "pages/index.html#200", previous: "", next: "" });
  expect(section.family).toEqual({ parent: "", previous: "", next: "" });
});

function contextMenu(page, element, init = {}) {
  const event = new page.window.MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 40, clientY: 60, ...init });
  element.dispatchEvent(event);
  return event;
}

test("a right click selects the element and asks the editor for its menu instead of the browser's", () => {
  const page = platformPage();
  const event = contextMenu(page, page.doc.querySelector("p"));
  expect(event.defaultPrevented).toBe(true);
  expect(page.messages("vz:select").map((m) => m.node)).toEqual(["pages/index.html#300"]);
  expect(page.messages("vz:context")).toEqual([{ type: "vz:context", node: "pages/index.html#300", x: 40, y: 60 }]);
  contextMenu(page, page.doc.body);
  expect(page.messages("vz:context").at(-1)).toEqual({ type: "vz:context", node: "", x: 40, y: 60 });
});

test("the keyboard opens the same menu for the picked element", () => {
  const page = platformPage();
  click(page, page.doc.querySelector("p"));
  key(page, page.doc.body, { key: "F10", shiftKey: true });
  key(page, page.doc.body, { key: "ContextMenu" });
  const asked = page.messages("vz:context");
  expect(asked.map((m) => m.node)).toEqual(["pages/index.html#300", "pages/index.html#300"]);
});

test("never offers the editor menu while text is being edited or outside select mode", () => {
  const page = setup();
  const heading = editing(page);
  expect(contextMenu(page, heading).defaultPrevented).toBe(false);
  key(page, heading, { key: "ContextMenu" });
  page.fromEditor({ type: "vz:mode", mode: "preview" });
  expect(contextMenu(page, page.doc.querySelector("section")).defaultPrevented).toBe(false);
  expect(page.messages("vz:context")).toEqual([]);
});

const pause = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function boxed(element, box) {
  element.getBoundingClientRect = () => ({ x: box.x, y: box.y, left: box.x, top: box.y, width: box.width, height: box.height, right: box.x + box.width, bottom: box.y + box.height });
}

test("keeps the editor's copy of the selection box in step when the page settles after a live change", async () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  const section = page.doc.querySelector("section");
  boxed(section, { x: 0, y: 100, width: 800, height: 300 });
  click(page, section);
  applyChange(page, "e1", "pages/index.html#90", { kind: "classes", add: ["w-1/2"], remove: [] });
  boxed(section, { x: 0, y: 100, width: 400, height: 300 });
  await pause(80);
  expect(page.messages("vz:styles").at(-1).rect).toEqual({ x: 0, y: 100, width: 400, height: 300 });
  const sent = page.messages("vz:styles").length;
  await pause(80);
  expect(page.messages("vz:styles").length).toBe(sent);
});

test("lands a new frame with the selected element where it was on screen, not only at the old scroll", async () => {
  const page = setup();
  boxed(h1(page), { x: 0, y: 105, width: 300, height: 40 });
  page.fromEditor({ type: "vz:scroll-y", y: 320, anchor: { node: "pages/index.html#10", top: 100 } });
  await announced(page, "vz:scroll");
  expect(page.window.scrollY).toBe(325);
  expect(page.messages("vz:scroll").at(-1)).toEqual({ type: "vz:scroll", y: 325 });
});

test("leaves the Tailwind compiler source out of renders and document snapshots, keeping the compiled styles", async () => {
  const shots = fakeScreenshot();
  const page = captureSetup(shots);
  const source = page.doc.createElement("style");
  source.setAttribute("type", "text/tailwindcss");
  source.textContent = '@import "tailwindcss/theme" layer(theme);';
  const compiler = page.doc.createElement("script");
  compiler.setAttribute("src", "/assets/vendor/vz-tailwind-browser-abc.js");
  const compiled = page.doc.createElement("style");
  compiled.textContent = ".p-4{padding:1rem}";
  page.doc.head.append(source, compiler, compiled);
  page.fromEditor({ type: "vz:capture", id: "d2", kind: "document" });
  const answer = await page.reply("d2");
  expect(answer.html).not.toContain("text/tailwindcss");
  expect(answer.html).not.toContain("tailwindcss/theme");
  expect(answer.html).toContain(".p-4{padding:1rem}");
  page.fromEditor({ type: "vz:capture", id: "i2", kind: "image", height: 400 });
  await page.reply("i2");
  const { filter } = shots.calls.at(-1).options;
  expect(filter(source)).toBe(false);
  expect(filter(compiler)).toBe(false);
  expect(filter(compiled)).toBe(true);
  expect(filter(page.doc.body)).toBe(true);
});

const frames = (page, count = 2) =>
  new Promise((resolve) => {
    const step = (left) => (left <= 0 ? resolve() : page.window.requestAnimationFrame(() => step(left - 1)));
    step(count);
  });

function boxAt(element, x, y, width, height) {
  element.getBoundingClientRect = () => ({ left: x, top: y, x, y, width, height, right: x + width, bottom: y + height });
}

function layout(page, html) {
  page.doc.body.innerHTML = html;
  page.fromEditor({ type: "vz:mode", mode: "select" });
}

test("scrolling sends at most one position per animation frame and reuses the outline boxes instead of rebuilding them", async () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  click(page, h1(page));
  page.fromEditor({ type: "vz:highlight", nodes: [{ node: "pages/index.html#10", kind: "selection", item: null }] });
  await frames(page);
  const box = page.toolbar().querySelector(".box.selection");
  const before = page.messages("vz:scroll").length;
  const scrollBy = (base) => {
    for (let i = 1; i <= 6; i++) {
      page.window.scrollTo(0, base + i * 10);
      page.window.dispatchEvent(new page.window.Event("scroll"));
    }
  };
  scrollBy(0);
  expect(page.messages("vz:scroll").length).toBe(before);
  await frames(page, 1);
  expect(page.messages("vz:scroll").length).toBe(before + 1);
  expect(page.messages("vz:scroll").at(-1)).toEqual({ type: "vz:scroll", y: page.window.scrollY });
  scrollBy(100);
  await frames(page, 1);
  expect(page.messages("vz:scroll").length).toBe(before + 2);
  expect(page.toolbar().querySelector(".box.selection")).toBe(box);
});

test("a class or attribute change on a looped element changes every copy at once and a revert restores them all", () => {
  const page = setup();
  layout(page, REPEATED + `<p data-vz-node="pages/index.html#10" class="py-2">Só</p>`);
  const copies = Array.from(page.doc.querySelectorAll(`[data-vz-node="src/App.jsx#40"]`));
  const single = page.doc.querySelector(`[data-vz-node="pages/index.html#10"]`);
  click(page, copies[1]);
  applyChange(page, "c1", "src/App.jsx#40", { kind: "classes", add: ["text-brand"], remove: [] }, 1);
  applyChange(page, "c2", "src/App.jsx#40", { kind: "attr", name: "title", value: "Serviço" }, 1);
  expect(copies.map((copy) => copy.classList.contains("text-brand"))).toEqual([true, true]);
  expect(copies.map((copy) => copy.getAttribute("title"))).toEqual(["Serviço", "Serviço"]);
  expect(single.className).toBe("py-2");
  page.fromEditor({ type: "vz:revert", id: "c1" });
  page.fromEditor({ type: "vz:revert", id: "c2" });
  expect(copies.map((copy) => copy.classList.contains("text-brand"))).toEqual([false, false]);
  expect(copies.map((copy) => copy.hasAttribute("title"))).toEqual([false, false]);
});

test("a refused change on a looped element leaves every copy untouched", () => {
  const page = setup();
  layout(page, REPEATED);
  applyChange(page, "x1", "src/App.jsx#40", { kind: "attr", name: "onclick", value: "x()" }, 0);
  expect(Array.from(page.doc.querySelectorAll("p")).some((copy) => copy.hasAttribute("onclick"))).toBe(false);
  expect(page.messages("vz:apply-refused")).toEqual([{ type: "vz:apply-refused", id: "x1", reason: "attribute_forbidden" }]);
});

test("an icon component rendered as svg is selected on its own, named as an icon, and takes live class changes", () => {
  const page = setup();
  layout(page, `<div data-vz-node="src/pages/Home.jsx#20"><span data-vz-node="src/pages/Home.jsx#40"><svg data-vz-inst="src/pages/Home.jsx#60" class="size-6"><path d="M0 0"></path></svg></span></div>`);
  click(page, page.doc.querySelector("path"));
  expect(page.messages("vz:select").at(-1)).toMatchObject({ node: "src/pages/Home.jsx#60", tag: "svg", kind: "icon", family: { parent: "src/pages/Home.jsx#40", previous: "", next: "" } });
  applyChange(page, "i1", "src/pages/Home.jsx#60", { kind: "classes", add: ["size-8"], remove: ["size-6"] });
  expect(page.doc.querySelector("svg").getAttribute("class")).toBe("size-8");
  click(page, page.doc.querySelector("span"));
  expect(page.messages("vz:select").at(-1)).toMatchObject({ node: "src/pages/Home.jsx#40" });
  expect(page.messages("vz:select").at(-1).kind).toBeUndefined();
});

const CARDS = `<section data-vz-node="pages/index.html#0">
<div data-vz-node="pages/index.html#20" class="row"><article data-vz-node="pages/index.html#40">A</article><article data-vz-node="pages/index.html#60">B</article></div>
<div data-vz-node="pages/index.html#90" class="empty"></div>
</section>`;

function cardsLayout(page) {
  layout(page, CARDS);
  const [section, row, empty] = [page.doc.querySelector("section"), page.doc.querySelector(".row"), page.doc.querySelector(".empty")];
  const [a, b] = page.doc.querySelectorAll("article");
  boxAt(section, 0, 0, 800, 600);
  boxAt(row, 0, 50, 800, 200);
  boxAt(a, 0, 50, 400, 200);
  boxAt(b, 400, 50, 400, 200);
  boxAt(empty, 0, 300, 800, 200);
  row.style.display = "flex";
  page.doc.elementsFromPoint = (x, y) =>
    [a, b, empty]
      .filter((element) => {
        const box = element.getBoundingClientRect();
        return x >= box.left && x <= box.right && y >= box.top && y <= box.bottom;
      })
      .concat(y >= 50 && y <= 250 ? [row] : [], [section, page.doc.body, page.doc.documentElement]);
  return { section, row, empty, a, b };
}

test("a probe finds the slot between siblings along the container's flow and draws the marker", async () => {
  const page = setup();
  cardsLayout(page);
  page.fromEditor({ type: "vz:probe", x: 395, y: 150, moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#20", anchor: "pages/index.html#60", place: "before" });
  page.fromEditor({ type: "vz:probe", x: 200, y: 150, moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#40", tag: "article", place: "into" });
  page.fromEditor({ type: "vz:probe", x: 395, y: 150, moving: "" });
  expect(page.messages("vz:drop").at(-1)).toEqual({ type: "vz:drop", slot: { container: "pages/index.html#20", tag: "div", anchor: "pages/index.html#60", place: "before", axis: "x" } });
  page.fromEditor({ type: "vz:probe", x: 790, y: 150, moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ anchor: "pages/index.html#60", place: "after", axis: "x" });
  page.fromEditor({ type: "vz:probe", x: 400, y: 400, moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toEqual({ container: "pages/index.html#90", tag: "div", anchor: "pages/index.html#90", place: "into", axis: "y" });
  await frames(page);
  expect(page.toolbar().querySelector(".drop-line").style.display).toBe("block");
  page.fromEditor({ type: "vz:probe-end" });
  await frames(page);
  expect(page.toolbar().querySelector(".drop-line").style.display).toBe("none");
});

test("a probe near a container's edge offers the slot beside it in the parent", () => {
  const page = setup();
  cardsLayout(page);
  page.fromEditor({ type: "vz:probe", x: 400, y: 303, moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#0", tag: "section", anchor: "pages/index.html#90", place: "before" });
});

test("a moving element can never land inside itself and is left out of the slots", () => {
  const page = setup();
  const { row } = cardsLayout(page);
  page.fromEditor({ type: "vz:probe", x: 100, y: 150, moving: "pages/index.html#20" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#20", refused: "inside_self" });
  page.fromEditor({ type: "vz:probe", x: 400, y: 400, moving: "pages/index.html#40" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ anchor: "pages/index.html#90", place: "into" });
  expect(row.children.length).toBe(2);
});

test("the keyboard starts right after the selection, then walks next, previous, out and in", () => {
  const page = setup();
  const { a } = cardsLayout(page);
  click(page, a);
  page.fromEditor({ type: "vz:probe-step", direction: "next", moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#20", anchor: "pages/index.html#60", place: "before" });
  page.fromEditor({ type: "vz:probe-step", direction: "next", moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#20", anchor: "pages/index.html#60", place: "after" });
  page.fromEditor({ type: "vz:probe-step", direction: "previous", moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ anchor: "pages/index.html#60", place: "before" });
  page.fromEditor({ type: "vz:probe-step", direction: "out", moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#0", anchor: "pages/index.html#90", place: "before" });
  page.fromEditor({ type: "vz:probe-step", direction: "in", moving: "" });
  expect(page.messages("vz:drop").at(-1).slot).toMatchObject({ container: "pages/index.html#90", place: "into" });
});

test("a long press on the selected element drags it: slots follow the pointer, release commits and the click after it is swallowed", async () => {
  const page = setup();
  const { a, b } = cardsLayout(page);
  click(page, a);
  a.dispatchEvent(new page.window.PointerEvent("pointerdown", { bubbles: true, button: 0, clientX: 100, clientY: 100 }));
  await new Promise((resolve) => setTimeout(resolve, 500));
  expect(page.messages("vz:drag-begin")).toEqual([{ type: "vz:drag-begin", node: "pages/index.html#40", item: null }]);
  page.doc.body.dispatchEvent(new page.window.PointerEvent("pointermove", { bubbles: true, clientX: 700, clientY: 150 }));
  expect(page.messages("vz:drop").at(-1)).toMatchObject({ slot: { anchor: "pages/index.html#60", place: "after" }, x: 700, y: 150 });
  page.doc.body.dispatchEvent(new page.window.PointerEvent("pointerup", { bubbles: true }));
  expect(page.messages("vz:drag-end")).toEqual([{ type: "vz:drag-end", commit: true }]);
  const selections = page.messages("vz:select").length;
  click(page, b);
  expect(page.messages("vz:select").length).toBe(selections);
});

test("Escape cancels a long press drag and a press that moves first never starts one", async () => {
  const page = setup();
  const { a } = cardsLayout(page);
  click(page, a);
  a.dispatchEvent(new page.window.PointerEvent("pointerdown", { bubbles: true, button: 0, clientX: 100, clientY: 100 }));
  page.doc.body.dispatchEvent(new page.window.PointerEvent("pointermove", { bubbles: true, clientX: 130, clientY: 100 }));
  await new Promise((resolve) => setTimeout(resolve, 500));
  expect(page.messages("vz:drag-begin")).toEqual([]);
  a.dispatchEvent(new page.window.PointerEvent("pointerdown", { bubbles: true, button: 0, clientX: 100, clientY: 100 }));
  await new Promise((resolve) => setTimeout(resolve, 500));
  key(page, page.doc.body, { key: "Escape" });
  expect(page.messages("vz:drag-end")).toEqual([{ type: "vz:drag-end", commit: false }]);
});

test("moves an element live into an empty container and back on revert, never into itself", () => {
  const page = setup();
  const { a, empty, row } = cardsLayout(page);
  applyChange(page, "m1", "pages/index.html#40", { kind: "move", anchor: "pages/index.html#90", place: "into" });
  expect(a.parentElement).toBe(empty);
  page.fromEditor({ type: "vz:revert", id: "m1" });
  expect(row.firstElementChild).toBe(a);
  applyChange(page, "m2", "pages/index.html#20", { kind: "move", anchor: "pages/index.html#40", place: "into" });
  expect(page.messages("vz:apply-refused").at(-1)).toEqual({ type: "vz:apply-refused", id: "m2", reason: "element_forbidden" });
});

test("hands copy, paste and duplicate shortcuts to the editor while selecting, never while editing text", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  expect(key(page, page.doc.body, { key: "c", ctrlKey: true }).defaultPrevented).toBe(true);
  key(page, page.doc.body, { key: "V", metaKey: true });
  key(page, page.doc.body, { key: "d", ctrlKey: true });
  key(page, page.doc.body, { key: "c", ctrlKey: true, shiftKey: true });
  expect(page.messages("vz:shortcut").map((message) => message.action)).toEqual(["copy", "paste", "duplicate"]);
  const heading = editing(page);
  key(page, heading, { key: "c", ctrlKey: true });
  expect(page.messages("vz:shortcut").length).toBe(3);
});

test("hover styles never match while the person designs, and come back on demand or in preview", async () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  await frames(page);
  const root = page.doc.documentElement;
  expect(root.hasAttribute("data-vz-design")).toBe(true);
  page.fromEditor({ type: "vz:hover-states", on: true });
  expect(root.hasAttribute("data-vz-design")).toBe(false);
  page.fromEditor({ type: "vz:hover-states", on: false });
  expect(root.hasAttribute("data-vz-design")).toBe(true);
  key(page, page.doc.body, { key: "Alt" });
  expect(root.hasAttribute("data-vz-design")).toBe(false);
  page.doc.body.dispatchEvent(new page.window.KeyboardEvent("keyup", { bubbles: true, key: "Alt" }));
  expect(root.hasAttribute("data-vz-design")).toBe(true);
  page.fromEditor({ type: "vz:mode", mode: "preview" });
  expect(root.hasAttribute("data-vz-design")).toBe(false);
});

test("rewrites the hover rules of every readable stylesheet, nested rules included, and skips the unreadable ones", () => {
  const nested = { selectorText: "&:hover" };
  const sheets = [
    { cssRules: [{ selectorText: ".card:hover" }, { selectorText: ".plain" }, { selectorText: ".x", cssRules: [nested] }, { cssRules: [{ selectorText: ".y:hover" }] }] },
    Object.defineProperty({}, "cssRules", { get: () => { throw new Error("cross origin"); } }),
  ];
  guardHoverSheets({ styleSheets: sheets });
  expect(sheets[0].cssRules.map((rule) => rule.selectorText)).toEqual([".card:hover:not(html[data-vz-design] *)", ".plain", ".x", undefined]);
  expect(nested.selectorText).toBe("&:hover:not(html[data-vz-design] *)");
  expect(sheets[0].cssRules[3].cssRules[0].selectorText).toBe(".y:hover:not(html[data-vz-design] *)");
});

test("guards every hover selector once, nested and grouped ones included", () => {
  expect(guardHover(".a:hover, .b:focus")).toBe(".a:hover:not(html[data-vz-design] *), .b:focus");
  expect(guardHover("&:hover")).toBe("&:hover:not(html[data-vz-design] *)");
  expect(guardHover(".x:is(:where(.group):hover *)")).toBe(".x:is(:where(.group):hover:not(html[data-vz-design] *) *)");
  expect(guardHover(guardHover(".a:hover::before"))).toBe(".a:hover:not(html[data-vz-design] *)::before");
});

test("a looped copy keeps its own instance when the editor selects it again after a rebuild, even without an item index", () => {
  const page = setup();
  layout(page, `<main><div class="card"><svg data-vz-inst="src/pages/Home.jsx#60"></svg></div><div class="card"><svg data-vz-inst="src/pages/Home.jsx#60"></svg></div></main>`);
  const [first, second] = page.doc.querySelectorAll("svg");
  page.fromEditor({ type: "vz:select-node", node: "src/pages/Home.jsx#60", item: null, path: elementPath(second) });
  expect(page.messages("vz:select").at(-1).path).toEqual(elementPath(second));
  page.fromEditor({ type: "vz:select-node", node: "src/pages/Home.jsx#60", item: null, path: elementPath(first) });
  expect(page.messages("vz:select").at(-1).path).toEqual(elementPath(first));
});

test("a selection box is never sent measured at a scroll the editor was not told about", () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  page.window.scrollTo(0, 240);
  click(page, h1(page));
  const types = page.sent.map((entry) => entry.message.type);
  expect(types.lastIndexOf("vz:scroll")).toBeLessThan(types.lastIndexOf("vz:select"));
  expect(page.messages("vz:scroll").at(-1)).toEqual({ type: "vz:scroll", y: page.window.scrollY });
});

test("a pointer drop marker clears itself once the page gets the pointer back, so an abandoned drag never leaves a line", async () => {
  const page = setup();
  cardsLayout(page);
  page.fromEditor({ type: "vz:probe", x: 400, y: 400, moving: "" });
  await frames(page);
  expect(page.toolbar().querySelector(".drop-box").style.display).toBe("block");
  page.doc.body.dispatchEvent(new page.window.PointerEvent("pointermove", { bubbles: true, clientX: 10, clientY: 10 }));
  await frames(page);
  expect(page.toolbar().querySelector(".drop-box").style.display).toBe("none");
  expect(page.toolbar().querySelector(".drop-line").style.display).toBe("none");
});

test("after a scroll the hover outline moves to whatever is now under the still pointer", async () => {
  const page = setup();
  page.fromEditor({ type: "vz:mode", mode: "select" });
  const heading = h1(page);
  const section = page.doc.querySelector("section");
  heading.dispatchEvent(new page.window.PointerEvent("pointermove", { bubbles: true, clientX: 50, clientY: 50 }));
  expect(page.messages("vz:hover").at(-1).node).toBe("pages/index.html#10");
  page.doc.elementFromPoint = () => section;
  page.window.scrollTo(0, 300);
  page.window.dispatchEvent(new page.window.Event("scroll"));
  await frames(page, 1);
  expect(page.messages("vz:hover").at(-1).node).toBe("pages/index.html#90");
});

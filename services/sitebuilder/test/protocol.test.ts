import { describe, expect, test } from "bun:test";
import { decodeBuild, decodeExecute, RequestInvalid } from "../src/protocol";

const project = { stack: "html", files: { "site.json": { content: "{}", rev: 1 } } };

describe("decodeExecute", () => {
  test("accepts the documented shape", () => {
    const decoded = decodeExecute({
      project,
      operations: [{ op: "set_knob", name: "Topo", value: 2.5 }, { op: "insert_section", at: { page: "pages/index.html" }, rev: 1, library_key: "faq" }],
      texts: { "0": [{ text: "a", bold: true, link: { href: "https://x" } }] },
      media: { m1: "https://cdn.example/a.webp" },
      sections: { faq: "<section></section>" },
      bases: { "pages/index.html": [{ rev: 1, content: "<p></p>" }] },
    });
    expect(decoded.operations[1].at).toEqual({ page: "pages/index.html" });
  });

  test.each([
    [{ project, operations: [] }],
    [{ project, operations: [{ op: "paint" }] }],
    [{ project, operations: [{ op: "remove", node: "a#0", rev: 0 }] }],
    [{ project, operations: [{ op: "remove", node: "a#0", rev: 1, reason: "x" }] }],
    [{ project, operations: [{ op: "remove", node: "a#0", rev: 1 }], media: { m: "javascript:alert(1)" } }],
    [{ project: { stack: "vue", files: {} }, operations: [{ op: "remove" }] }],
    [{ project, operations: [{ op: "remove" }], extra: true }],
  ])("refuses %j", (body) => {
    expect(() => decodeExecute(body)).toThrow(RequestInvalid);
  });
});

describe("decodeBuild", () => {
  const shell = { api: "https://api.example", pages: { "/": { title: "x", noindex: true } }, integrations: {}, forms: {} };

  test("accepts preview and publish", () => {
    expect(decodeBuild({ mode: "publish", project, shell, runtime: { "pow.js": "" } }).mode).toBe("publish");
  });

  test.each([
    [{ mode: "draft", project, shell, runtime: {} }],
    [{ mode: "preview", project, shell: { ...shell, api: "javascript:x" }, runtime: {} }],
    [{ mode: "preview", project, shell: { ...shell, pages: { "/": { robots: "x" } } }, runtime: {} }],
    [{ mode: "preview", project, shell, runtime: { "pow.js": 1 } }],
  ])("refuses %j", (body) => {
    expect(() => decodeBuild(body)).toThrow(RequestInvalid);
  });
});

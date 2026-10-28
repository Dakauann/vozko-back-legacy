import { describe, expect, test } from "bun:test";
import { escapeJsxText, jsxAttributeValue, plainText, renderRuns, type Run } from "../src/execute/inline";

describe("renderRuns", () => {
  const runs: Run[] = [
    { text: "Olá " },
    { text: "mundo", bold: true },
    { text: " e " },
    { text: "veja", italic: true, link: { href: "https://a.b/?x=1&y=\"2\"" } },
    { text: " aqui", bold: true, link: { href: "https://a.b/?x=1&y=\"2\"" } },
    { text: " 1 < 2 {ok}", underline: true },
  ];

  test("html escapes entities and nests marks inside one link", () => {
    expect(renderRuns(runs, "html")).toBe(
      'Olá <strong>mundo</strong> e <a href="https://a.b/?x=1&amp;y=&quot;2&quot;"><em>veja</em><strong> aqui</strong></a><u> 1 &lt; 2 {ok}</u>',
    );
  });

  test("jsx escapes braces and angle brackets", () => {
    expect(renderRuns(runs, "jsx")).toBe(
      'Olá <strong>mundo</strong> e <a href={"https://a.b/?x=1&y=\\"2\\""}><em>veja</em><strong> aqui</strong></a><u> 1 {"<"} 2 {"{"}ok{"}"}</u>',
    );
  });

  test("line breaks become br", () => {
    expect(renderRuns([{ text: "a\nb" }], "html")).toBe("a<br>b");
    expect(renderRuns([{ text: "a\nb" }], "jsx")).toBe("a<br />b");
  });

  test("bold italic underline nest strong em u", () => {
    expect(renderRuns([{ text: "x", bold: true, italic: true, underline: true }], "html")).toBe("<strong><em><u>x</u></em></strong>");
  });
});

describe("jsx helpers", () => {
  test("ampersands before entities are escaped, plain ones kept", () => {
    expect(escapeJsxText("Tom & Jerry &amp; co")).toBe('Tom & Jerry {"&"}amp; co');
  });

  test("attribute values use a string literal only when safe", () => {
    expect(jsxAttributeValue("lazy")).toBe('"lazy"');
    expect(jsxAttributeValue('a"b')).toBe('{"a\\"b"}');
  });

  test("plain text refuses marks", () => {
    expect(plainText([{ text: "a" }, { text: "b" }])).toBe("ab");
    expect(plainText([{ text: "a", bold: true }])).toBeNull();
  });
});

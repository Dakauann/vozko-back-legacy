import { describe, expect, test } from "bun:test";
import { locateEdit } from "../src/execute/edit";

function apply(content: string, old: string, replacement: string, replaceAll = false): string {
  const found = locateEdit(content, old, replaceAll);
  if (!found.ok) throw new Error(found.code);
  let out = "";
  let cursor = 0;
  for (const [start, end] of found.ranges) {
    out += content.slice(cursor, start) + replacement;
    cursor = end;
  }
  return out + content.slice(cursor);
}

describe("locateEdit", () => {
  test("exact single match", () => {
    expect(apply("<h1>Oi</h1>", "Oi", "Olá")).toBe("<h1>Olá</h1>");
  });

  test("typographic quotes, dashes and spaces match their plain forms", () => {
    const content = 'const a = "Oi - tudo bem";\n';
    expect(apply(content, "const a = “Oi — tudo bem”;", "const a = 1;")).toBe("const a = 1;\n");
  });

  test("trailing whitespace differences still match whole lines", () => {
    const content = "<div>   \n  <p>x</p>\t\n</div>\n";
    expect(apply(content, "<div>\n  <p>x</p>\n", "<section>\n")).toBe("<section>\n</div>\n");
  });

  test("ambiguous without replace_all", () => {
    const found = locateEdit("a\nb\na\n", "a", false);
    expect(found).toEqual({ ok: false, code: "edit_ambiguous", detail: "2 matches at lines 1, 3" });
  });

  test("replace_all replaces every match", () => {
    expect(apply("Logo Logo", "Logo", "Marca", true)).toBe("Marca Marca");
  });

  test("not found carries the nearest context", () => {
    const found = locateEdit("<header>\n  <h1 class=\"title\">Clinica</h1>\n</header>\n", "<h1 class=\"titulo\">Clinica</h1>", false);
    expect(found.ok).toBe(false);
    if (found.ok) return;
    expect(found.code).toBe("edit_not_found");
    expect(found.detail).toContain("nearest match at line 2");
    expect(found.detail).toContain('<h1 class="title">Clinica</h1>');
  });
});

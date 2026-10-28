import { describe, expect, test } from "bun:test";
import { mapOffset, OverlapError, Patcher, type Splice } from "../src/execute/patcher";
import { diffSplices } from "../src/execute/diff";

function applySplices(before: string, after: string, splices: Splice[]): string {
  let out = "";
  let cursor = 0;
  let added = 0;
  for (const splice of splices) {
    out += before.slice(cursor, splice.start);
    const at = splice.start + added;
    out += after.slice(at, at + splice.length);
    added += splice.length - (splice.end - splice.start);
    cursor = splice.end;
  }
  return out + before.slice(cursor);
}

describe("Patcher", () => {
  test("replaces, removes and inserts with original coordinates", () => {
    const p = new Patcher("<a><b></b><c></c></a>");
    p.replace(3, 6, "<x>").remove(10, 17).insertBefore(3, "<i></i>").insertAfter(17, "<z></z>");
    const { content, splices } = p.result();
    expect(content).toBe("<a><i></i><x></b><z></z></a>");
    expect(splices).toEqual([
      { start: 3, end: 3, length: 7 },
      { start: 3, end: 6, length: 3 },
      { start: 10, end: 17, length: 0 },
      { start: 17, end: 17, length: 7 },
    ]);
  });

  test("refuses overlapping edits", () => {
    const p = new Patcher("abcdef");
    p.replace(1, 4, "x");
    expect(() => p.replace(3, 5, "y")).toThrow(OverlapError);
    expect(() => p.insertBefore(2, "y")).toThrow(OverlapError);
  });

  test("an unchanged patcher reports no edits", () => {
    const p = new Patcher("abc");
    expect(p.changed).toBe(false);
    expect(p.result()).toEqual({ content: "abc", splices: [] });
  });
});

describe("mapOffset", () => {
  const splices: Splice[] = [
    { start: 3, end: 3, length: 7 },
    { start: 10, end: 17, length: 0 },
  ];
  test.each([
    [0, 0],
    [3, 10],
    [5, 12],
    [10, null],
    [16, null],
    [17, 17],
    [20, 20],
  ])("offset %p maps to %p", (offset, expected) => {
    expect(mapOffset(offset, splices)).toBe(expected);
  });
});

describe("diffSplices", () => {
  test.each([
    ["same", "abc\ndef\n", "abc\ndef\n"],
    ["one line", "<h1>Oi</h1>\n<p>x</p>\n", "<h1>Olá</h1>\n<p>x</p>\n"],
    ["insert lines", "a\nb\n", "a\nnew\nb\n"],
    ["remove all", "a\nb\n", ""],
    ["several", "<a>\n  <b>one</b>\n  <c>two</c>\n</a>\n", "<a>\n  <b>uno</b>\n  <c>two</c>\n  <d>three</d>\n</a>\n"],
  ])("%s rebuilds the new text", (_name, before, after) => {
    const splices = diffSplices(before, after);
    expect(applySplices(before, after, splices)).toBe(after);
  });

  test("offsets outside the edited region survive", () => {
    const before = "<h1>Oi</h1>\n<p>x</p>\n";
    const after = "<h1>Olá mundo</h1>\n<p>x</p>\n";
    const splices = diffSplices(before, after);
    expect(mapOffset(0, splices)).toBe(0);
    expect(mapOffset(before.indexOf("<p>"), splices)).toBe(after.indexOf("<p>"));
  });
});

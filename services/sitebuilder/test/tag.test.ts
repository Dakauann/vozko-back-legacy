import { describe, expect, test } from "bun:test";
import { parseHtmlFragment, serializeFragment, tagHtml, tagJsx } from "../src/build/tag";

describe("tagJsx", () => {
  test("tags intrinsic elements, component call sites and literal map items", () => {
    const source =
      'const items = ["a", "b"];\nexport default function Home() {\n  return <main><Hero title="x" />{items.map(item => <li key={item}><b>{item}</b></li>)}<motion.div /></main>;\n}\n';
    const out = tagJsx(source, "src/Home.jsx");
    const at = (needle: string) => source.indexOf(needle);
    expect(out).toContain(`<main data-vz-node="src/Home.jsx#${at("<main")}">`);
    expect(out).toContain(`<Hero data-vz-inst="src/Home.jsx#${at("<Hero")}" title="x" />`);
    expect(out).toContain(`items.map((item, __vzI) => <li data-vz-node="src/Home.jsx#${at("<li")}" data-vz-i={__vzI} key={item}>`);
    expect(out).toContain(`<b data-vz-node="src/Home.jsx#${at("<b>")}" data-vz-i={__vzI}>`);
    expect(out).toContain(`<motion.div data-vz-node="src/Home.jsx#${at("<motion.div")}" />`);
  });

  test("reuses an existing index parameter and skips maps over dynamic arrays", () => {
    const source = "const xs = [1];\nfunction A({ ys }) {\n  return <ul>{xs.map((x, k) => <li key={k} />)}{ys.map((y) => <p key={y} />)}</ul>;\n}\n";
    const out = tagJsx(source, "src/A.jsx");
    expect(out).toContain("data-vz-i={k}");
    expect(out).not.toContain("__vzI");
  });
});

describe("tagHtml", () => {
  test("adds the node id of every source element", () => {
    const source = '<section class="a">\n  <h1>Oi</h1>\n</section>\n';
    const fragment = parseHtmlFragment(source, true);
    tagHtml(fragment, "pages/index.html");
    expect(serializeFragment(fragment)).toBe(`<section class="a" data-vz-node="pages/index.html#0">\n  <h1 data-vz-node="pages/index.html#${source.indexOf("<h1")}">Oi</h1>\n</section>\n`);
  });
});

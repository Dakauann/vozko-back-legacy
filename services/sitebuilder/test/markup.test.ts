import { describe, expect, test } from "bun:test";
import { inlineOnly, parseHtml } from "../src/markup/html";
import { parseJsx } from "../src/markup/jsx";
import { htmlToJsx } from "../src/markup/html-to-jsx";
import { lineBlock, reindent } from "../src/markup/element";

describe("parseHtml", () => {
  const content = '<section class="hero py-20">\n  <h1 class="text-4xl">Oi <b>x</b></h1>\n  <img src="/media/a.webp" alt=\'Foto\'>\n</section>\n';
  const doc = parseHtml(content);

  test("elements carry source ranges and attribute value ranges", () => {
    const h1 = doc.at(content.indexOf("<h1"))!;
    expect(h1.tag).toBe("h1");
    expect(content.slice(h1.start, h1.end)).toBe('<h1 class="text-4xl">Oi <b>x</b></h1>');
    expect(content.slice(h1.innerStart!, h1.innerEnd!)).toBe("Oi <b>x</b>");
    const cls = h1.attrs[0];
    expect(content.slice(cls.valueStart, cls.valueEnd)).toBe("text-4xl");
    expect(inlineOnly(h1)).toBe(true);
    const img = doc.at(content.indexOf("<img"))!;
    expect(img.innerStart).toBeNull();
    expect(img.attrs.map((a) => [a.name, a.value])).toEqual([["src", "/media/a.webp"], ["alt", "Foto"]]);
    expect(h1.parent?.tag).toBe("section");
    expect(inlineOnly(doc.at(0)!)).toBe(false);
  });

  test("offsets that are not element starts resolve to nothing", () => {
    expect(doc.at(30)).toBeUndefined();
  });
});

describe("parseJsx", () => {
  const content = 'const items = ["A", "B"];\nexport default function Home() {\n  return <ul className="list">{items.map((item, i) => <li key={i}>{item}</li>)}<Hero title="Oi" /></ul>;\n}\n';
  const doc = parseJsx(content, "src/pages/Home.jsx");

  test("jsx elements expose tags, literals and placement", () => {
    const ul = doc.at(content.indexOf("<ul"))!;
    expect(ul.tag).toBe("ul");
    expect(ul.placeable).toBe(false);
    expect(ul.attrs[0]).toMatchObject({ name: "className", value: "list", literal: true });
    const li = doc.at(content.indexOf("<li"))!;
    expect(li.placeable).toBe(false);
    expect(li.parent?.tag).toBe("ul");
    const hero = doc.at(content.indexOf("<Hero"))!;
    expect(hero.component).toBe(true);
    expect(hero.placeable).toBe(true);
    expect(hero.innerStart).toBeNull();
  });
});

describe("htmlToJsx", () => {
  test("renames attributes, closes void tags and converts styles", () => {
    expect(htmlToJsx('<section class="py-8" style="margin-top: 2px; --x: 1"><label for="a">Nome {x}</label><input id="a" required><img src="/a.png" alt=""></section>')).toBe(
      '<section className="py-8" style={{ marginTop: "2px", "--x": "1" }}><label htmlFor="a">Nome {"{"}x{"}"}</label><input id="a" required /><img src="/a.png" alt="" /></section>',
    );
  });

  test("drops inline event handlers", () => {
    expect(htmlToJsx('<button onclick="x()">Ok</button>')).toBe("<button>Ok</button>");
  });
});

describe("line blocks", () => {
  test("an element alone on its lines takes the whole lines", () => {
    const content = "<ul>\n  <li>a</li>\n  <li>b</li>\n</ul>\n";
    const start = content.indexOf("<li>a");
    const block = lineBlock(content, start, start + "<li>a</li>".length);
    expect(block).toEqual({ start: 5, end: 18, whole: true, indent: "  " });
    expect(reindent("  <li>a</li>\n", "  ", "    ")).toBe("    <li>a</li>\n");
  });

  test("an inline element keeps its exact range", () => {
    const content = "<p>a <b>x</b> c</p>";
    expect(lineBlock(content, 5, 13).whole).toBe(false);
  });
});

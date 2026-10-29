import { describe, expect, test } from "bun:test";
import { execute } from "../src/execute/executor";
import type { ExecuteRequest, Operation, StepPatch } from "../src/execute/types";
import type { Project } from "../src/project";
import type { Run } from "../src/execute/inline";

const htmlProject: Project = {
  stack: "html",
  files: {
    "site.json": { content: '{"name":"Clinica","routes":[{"path":"/","page":"pages/index.html"}]}', rev: 1 },
    "theme.css": { content: '@import "tailwindcss";\n@theme {\n  --color-brand: #0f766e;\n}\n:root {\n  --hero-gap: 4rem;\n}\n', rev: 1 },
    "knobs.json": { content: '{"knobs":[{"name":"Topo","variable":"--hero-gap","kind":"range","min":0,"max":8,"step":0.5,"unit":"rem"}]}', rev: 1 },
    "pages/index.html": {
      content: '<section class="hero py-20">\n  <h1 class="text-4xl">Oi</h1>\n  <p>Texto</p>\n  <img src="/media/a.webp" alt="Foto">\n</section>\n',
      rev: 3,
    },
  },
};

const reactProject: Project = {
  stack: "react",
  files: {
    "site.json": { content: '{"name":"Clinica","routes":[{"path":"/","page":"src/pages/Home.jsx"}]}', rev: 1 },
    "src/index.css": { content: '@import "tailwindcss";\n@theme {\n  --color-brand: #0f766e;\n}\n', rev: 1 },
    "src/main.jsx": { content: 'import "./index.css";\n', rev: 1 },
    "src/pages/Home.jsx": {
      content:
        'import { Hero } from "../components/Hero";\nconst items = [{ title: "Implante" }, { title: "Clareamento" }];\nexport default function Home() {\n  return (\n    <main className="p-4">\n      <Hero title="Bem-vindo" />\n      <ul>\n        {items.map((item, i) => (\n          <li key={i}>{item.title}</li>\n        ))}\n      </ul>\n      <h2 className={cn("text-2xl", open && "underline")}>Serviços</h2>\n    </main>\n  );\n}\n',
      rev: 2,
    },
    "src/components/Hero.jsx": {
      content: 'export function Hero({ title }) {\n  return (\n    <section>\n      <h1 className="text-4xl">{title}</h1>\n      <img src="/a.png" alt="" srcSet="/a.png 1x" />\n    </section>\n  );\n}\n',
      rev: 1,
    },
  },
};

function request(project: Project, operations: Operation[], extra: Partial<ExecuteRequest> = {}): ExecuteRequest {
  return { project, operations, texts: {}, media: {}, sections: {}, bases: {}, ...extra };
}

function node(project: Project, path: string, needle: string, from = 0): string {
  const at = project.files[path].content.indexOf(needle, from);
  if (at < 0) throw new Error(`missing ${needle}`);
  return `${path}#${at}`;
}

async function steps(req: ExecuteRequest): Promise<StepPatch[]> {
  const result = await execute(req);
  if (!("steps" in result)) throw new Error(JSON.stringify(result));
  return result.steps;
}

async function final(req: ExecuteRequest, path: string): Promise<string> {
  const all = await steps(req);
  let content = req.project.files[path]?.content;
  for (const step of all) if (step.files?.[path] !== undefined) content = step.files[path];
  return content!;
}

const plain = (text: string): Run[] => [{ text }];

describe("execute on the html stack", () => {
  const page = "pages/index.html";

  test("set_text writes inline markup inside the element", async () => {
    const text: Run[] = [{ text: "Olá " }, { text: "mundo", bold: true }];
    const out = await steps(request(htmlProject, [{ op: "set_text", node: node(htmlProject, page, "<h1"), rev: 3, text: "Olá **mundo**" }], { texts: { "0": text } }));
    expect(out[0].files![page]).toContain('<h1 class="text-4xl">Olá <strong>mundo</strong></h1>');
    expect(out[0].focus).toBe(node(htmlProject, page, "<h1"));
  });

  test("set_text refuses an element holding blocks", async () => {
    const result = await execute(request(htmlProject, [{ op: "set_text", node: `${page}#0`, rev: 3, text: "x" }], { texts: { "0": plain("x") } }));
    expect(result).toEqual({ failures: [{ index: 0, code: "node_invalid", field: "node" }] });
  });

  test("set_classes merges with tailwind-merge for a breakpoint", async () => {
    const content = await final(request(htmlProject, [{ op: "set_classes", node: `${page}#0`, rev: 3, add: ["py-24"], remove: ["py-20"], breakpoint: "md" }]), page);
    expect(content.startsWith('<section class="hero py-20 md:py-24">')).toBe(true);
    const merged = await final(request(htmlProject, [{ op: "set_classes", node: `${page}#0`, rev: 3, add: ["py-24"] }]), page);
    expect(merged.startsWith('<section class="hero py-24">')).toBe(true);
  });

  test("set_classes refuses classes outside the design system", async () => {
    const result = await execute(request(htmlProject, [{ op: "set_classes", node: `${page}#0`, rev: 3, add: ["made-up-class"] }]));
    expect(result).toEqual({ failures: [{ index: 0, code: "class_invalid", field: "add" }] });
  });

  test("set_classes merges classes made from the project theme tokens", async () => {
    const themed: Project = {
      ...htmlProject,
      files: {
        ...htmlProject.files,
        "theme.css": { content: '@import "tailwindcss";\n@theme {\n  --text-hero: 5rem;\n  --color-ink: #111111;\n  --font-display: "Inter", sans-serif;\n  --shadow-soft: 0 1px 2px #0003;\n}\n', rev: 1 },
        "pages/index.html": { content: '<section class="py-20">\n  <h1 class="text-4xl text-ink font-serif shadow-lg">Oi</h1>\n  <h2 class="text-hero text-white">Ola</h2>\n</section>\n', rev: 3 },
      },
    };
    const h1 = node(themed, page, "<h1");
    const h2 = node(themed, page, "<h2");
    const sized = await final(request(themed, [{ op: "set_classes", node: h1, rev: 3, add: ["text-hero", "font-display", "shadow-soft"] }]), page);
    expect(sized).toContain('<h1 class="text-ink text-hero font-display shadow-soft">');
    const colored = await final(request(themed, [{ op: "set_classes", node: h2, rev: 3, add: ["text-ink"] }]), page);
    expect(colored).toContain('<h2 class="text-hero text-ink">');
  });

  test("an unexpected error in one operation is a coded failure without internals", async () => {
    const broken: Project = { ...htmlProject, files: { ...htmlProject.files, "theme.css": { content: '@import "tailwindcss";\n.card { @apply not-a-utility; }\n', rev: 1 } } };
    const result = await execute(request(broken, [{ op: "set_classes", node: `${page}#0`, rev: 3, add: ["py-24"] }, { op: "set_attr", node: `${page}#0`, rev: 3, name: "id", value: "topo" }]));
    expect(result).toEqual({ failures: [{ index: 0, code: "operation_failed" }], skipped: [{ index: 1, after: 0 }] });
    expect(JSON.stringify(result)).not.toContain("not-a-utility");
  });

  test("set_attr adds, replaces and removes", async () => {
    const img = node(htmlProject, page, "<img");
    const content = await final(
      request(htmlProject, [
        { op: "set_attr", node: img, rev: 3, name: "loading", value: "lazy" },
        { op: "set_attr", node: img, rev: 3, name: "alt", value: 'A "b"' },
        { op: "set_attr", node: node(htmlProject, page, "<h1"), rev: 3, name: "id", value: "topo" },
      ]),
      page,
    );
    expect(content).toContain('<img src="/media/a.webp" alt="A &quot;b&quot;" loading="lazy">');
    expect(content).toContain('<h1 class="text-4xl" id="topo">');
  });

  test("replace_image swaps src and alt", async () => {
    const content = await final(
      request(htmlProject, [{ op: "replace_image", node: node(htmlProject, page, "<img"), rev: 3, media_id: "m1", alt: "" }], { media: { m1: "https://cdn.example/m1.webp" } }),
      page,
    );
    expect(content).toContain('<img src="https://cdn.example/m1.webp" alt="">');
  });

  test("replace_image refuses unknown media", async () => {
    const result = await execute(request(htmlProject, [{ op: "replace_image", node: node(htmlProject, page, "<img"), rev: 3, media_id: "m9" }]));
    expect(result).toEqual({ failures: [{ index: 0, code: "media_invalid", field: "media_id" }] });
  });

  test("move, duplicate with a ref, insert after the ref and remove", async () => {
    const out = await steps(
      request(
        htmlProject,
        [
          { op: "duplicate", node: node(htmlProject, page, "<h1"), rev: 3, ref: "titulo" },
          { op: "move", node: "@titulo", rev: 3, after: node(htmlProject, page, "<img") },
          { op: "insert_section", at: { after: "@titulo" }, html: "<div>\n  <p>Novo</p>\n</div>", ref: "novo" },
          { op: "remove", node: node(htmlProject, page, "<p>") , rev: 3},
        ],
      ),
    );
    expect(out[3].files![page]).toBe(
      '<section class="hero py-20">\n  <h1 class="text-4xl">Oi</h1>\n  <img src="/media/a.webp" alt="Foto">\n  <h1 class="text-4xl">Oi</h1>\n  <div>\n    <p>Novo</p>\n  </div>\n</section>\n',
    );
    const second = out[1].files![page];
    expect(out[1].focus).toBe(`${page}#${second.lastIndexOf("<h1")}`);
  });

  test("insert_section from the library into a page", async () => {
    const content = await final(request(htmlProject, [{ op: "insert_section", at: { page }, rev: 3, library_key: "faq" }], { sections: { faq: "<section>FAQ</section>" } }), page);
    expect(content.endsWith("</section>\n<section>FAQ</section>\n")).toBe(true);
    const missing = await execute(request(htmlProject, [{ op: "insert_section", at: { page }, rev: 3, library_key: "nada" }]));
    expect(missing).toEqual({ failures: [{ index: 0, code: "library_key_invalid", field: "library_key" }] });
  });

  test("insert into an element goes in as the last child", async () => {
    const content = await final(request(htmlProject, [{ op: "insert_section", at: { into: `${page}#0` }, rev: 3, html: "<footer>Fim</footer>" }]), page);
    expect(content).toBe('<section class="hero py-20">\n  <h1 class="text-4xl">Oi</h1>\n  <p>Texto</p>\n  <img src="/media/a.webp" alt="Foto">\n  <footer>Fim</footer>\n</section>\n');
  });

  test("move into an element puts it last inside and refuses moving an element into itself", async () => {
    const withBox: Project = { ...htmlProject, files: { ...htmlProject.files, [page]: { content: '<section class="hero py-20">\n  <h1 class="text-4xl">Oi</h1>\n  <div class="grid"></div>\n</section>\n', rev: 3 } } };
    const content = await final(request(withBox, [{ op: "move", node: node(withBox, page, "<h1"), rev: 3, into: node(withBox, page, "<div") }]), page);
    expect(content).toBe('<section class="hero py-20">\n  <div class="grid"><h1 class="text-4xl">Oi</h1></div>\n</section>\n');
    const nested = await execute(request(withBox, [{ op: "move", node: `${page}#0`, rev: 3, into: node(withBox, page, "<div") }]));
    expect(nested).toEqual({ failures: [{ index: 0, code: "position_invalid", field: "into" }] });
    const voided = await execute(request(htmlProject, [{ op: "move", node: node(htmlProject, page, "<h1"), rev: 3, into: node(htmlProject, page, "<img") }]));
    expect(voided).toEqual({ failures: [{ index: 0, code: "position_invalid", field: "into" }] });
  });

  test("insert_section copies an element from another page at its own revision", async () => {
    const twoPages: Project = { ...htmlProject, files: { ...htmlProject.files, "pages/obrigado.html": { content: "<main>\n  <h1>Obrigado</h1>\n</main>\n", rev: 5 } } };
    const content = await final(
      request(twoPages, [{ op: "insert_section", at: { into: node(twoPages, "pages/obrigado.html", "<main") }, rev: 5, source: node(twoPages, page, "<p>"), source_rev: 3 }]),
      "pages/obrigado.html",
    );
    expect(content).toBe("<main>\n  <h1>Obrigado</h1>\n  <p>Texto</p>\n</main>\n");
    const gone = await execute(request(twoPages, [{ op: "insert_section", at: { page: "pages/obrigado.html" }, rev: 5, source: `${page}#7`, source_rev: 3 }]));
    expect(gone).toEqual({ failures: [{ index: 0, code: "node_stale", field: "source" }] });
  });

  test("theme token and knob", async () => {
    const out = await steps(
      request(htmlProject, [
        { op: "set_token", name: "--color-brand", value: "oklch(0.7 0.1 30)" },
        { op: "set_knob", name: "Topo", value: 2.5 },
      ]),
    );
    expect(out[1].files!["theme.css"]).toBe('@import "tailwindcss";\n@theme {\n  --color-brand: oklch(0.7 0.1 30);\n}\n:root {\n  --hero-gap: 2.5rem;\n}\n');
  });

  test("stale offsets move through earlier operations in the batch", async () => {
    const out = await steps(
      request(
        htmlProject,
        [
          { op: "insert_section", at: { before: `${page}#0` }, rev: 3, html: "<header>Topo</header>" },
          { op: "set_text", node: node(htmlProject, page, "<p>"), rev: 3, text: "Novo" },
        ],
        { texts: { "1": plain("Novo") } },
      ),
    );
    expect(out[1].files![page]).toContain("<p>Novo</p>");
    expect(out[1].files![page].startsWith("<header>Topo</header>\n<section")).toBe(true);
  });

  test("older revisions map through the supplied base content", async () => {
    const base = '<section class="hero py-20">\n  <h2>Antes</h2>\n  <h1>Oi</h1>\n  <p>Texto</p>\n  <img src="/media/a.webp" alt="Foto">\n</section>\n';
    const ok = await steps(
      request(htmlProject, [{ op: "set_text", node: `${page}#${base.indexOf("<p>")}`, rev: 2, text: "Velho" }], {
        texts: { "0": plain("Velho") },
        bases: { [page]: [{ rev: 2, content: base }] },
      }),
    );
    expect(ok[0].files![page]).toContain("<p>Velho</p>");
    const vanished = await execute(
      request(htmlProject, [{ op: "set_text", node: `${page}#${base.indexOf("<h2>")}`, rev: 2, text: "x" }], {
        texts: { "0": plain("x") },
        bases: { [page]: [{ rev: 2, content: base }] },
      }),
    );
    expect(vanished).toEqual({ failures: [{ index: 0, code: "node_stale", field: "node" }] });
    const unknown = await execute(request(htmlProject, [{ op: "set_text", node: `${page}#0`, rev: 1, text: "x" }], { texts: { "0": plain("x") } }));
    expect(unknown).toEqual({ failures: [{ index: 0, code: "node_stale", field: "node" }] });
  });

  test("an offset that is not an element start is stale", async () => {
    const result = await execute(request(htmlProject, [{ op: "remove", node: `${page}#5`, rev: 3 }]));
    expect(result).toEqual({ failures: [{ index: 0, code: "node_stale", field: "node" }] });
  });
});

describe("execute code operations", () => {
  const page = "pages/index.html";

  test("edit_file leniency and refusals with context", async () => {
    const out = await steps(request(htmlProject, [{ op: "edit_file", path: page, old: "<p>Texto</p>", new: "<p>Mais</p>" }]));
    expect(out[0]).toEqual({ op: "edit_file", index: 0, focus: page, files: { [page]: htmlProject.files[page].content.replace("Texto", "Mais") } });
    const result = await execute(request(htmlProject, [{ op: "edit_file", path: page, old: "<p>Textos</p>", new: "x" }]));
    expect("failures" in result && result.failures[0].code).toBe("edit_not_found");
    expect("failures" in result && result.failures[0].detail).toContain("<p>Texto</p>");
  });

  test("every failure is listed, dependents are skipped and nothing changes", async () => {
    const result = await execute(
      request(htmlProject, [
        { op: "edit_file", path: page, old: "nada", new: "x" },
        { op: "set_text", node: `${page}#0`, rev: 3, text: "y" },
        { op: "set_token", name: "--color-brand", value: "#000" },
        { op: "set_knob", name: "Inexistente", value: 1 },
      ]),
    );
    expect("failures" in result).toBe(true);
    if (!("failures" in result)) return;
    expect(result.failures.map((f) => [f.index, f.code])).toEqual([[0, "edit_not_found"], [3, "knob_unknown"]]);
    expect(result.skipped).toEqual([{ index: 1, after: 0 }]);
  });

  test("write, rename and delete produce step shapes", async () => {
    const out = await steps(
      request(htmlProject, [
        { op: "write_file", path: "pages/sobre.html", content: "<h1>Sobre</h1>\n" },
        { op: "rename_file", path: "pages/sobre.html", to: "pages/quem.html" },
        { op: "delete_file", path: "knobs.json" },
      ]),
    );
    expect(out).toEqual([
      { op: "write_file", index: 0, focus: "pages/sobre.html", files: { "pages/sobre.html": "<h1>Sobre</h1>\n" } },
      { op: "rename_file", index: 1, focus: "pages/quem.html", renamed: [{ from: "pages/sobre.html", to: "pages/quem.html" }] },
      { op: "delete_file", index: 2, focus: "knobs.json", deleted: ["knobs.json"] },
    ]);
  });
});

describe("execute on the react stack", () => {
  const home = "src/pages/Home.jsx";
  const hero = "src/components/Hero.jsx";

  test("array item text patches the literal in the source array", async () => {
    const content = await final(request(reactProject, [{ op: "set_text", node: node(reactProject, home, "<li"), rev: 2, item: 1, text: "Lentes" }], { texts: { "0": plain("Lentes") } }), home);
    expect(content).toContain('const items = [{ title: "Implante" }, { title: "Lentes" }];');
  });

  test("prop text resolves through the single call site", async () => {
    const out = await steps(request(reactProject, [{ op: "set_text", node: node(reactProject, hero, "<h1"), rev: 1, text: 'Olá "todos"' }], { texts: { "0": plain('Olá "todos"') } }));
    expect(out[0].files![home]).toContain('<Hero title={"Olá \\"todos\\""} />');
    expect(out[0].focus).toBe(node(reactProject, hero, "<h1"));
  });

  test("markup in a literal is refused", async () => {
    const result = await execute(request(reactProject, [{ op: "set_text", node: node(reactProject, hero, "<h1"), rev: 1, text: "**x**" }], { texts: { "0": [{ text: "x", bold: true }] } }));
    expect(result).toEqual({ failures: [{ index: 0, code: "text_markup_invalid", field: "text" }] });
  });

  test("jsx text is escaped", async () => {
    const content = await final(request(reactProject, [{ op: "set_text", node: node(reactProject, home, "<h2"), rev: 2, text: "a {b}" }], { texts: { "0": [{ text: "a {b} " }, { text: "c", italic: true }] } }), home);
    expect(content).toContain('>a {"{"}b{"}"} <em>c</em></h2>');
  });

  test("className literal and cn first argument take class edits", async () => {
    const out = await steps(
      request(reactProject, [
        { op: "set_classes", node: node(reactProject, home, "<h2"), rev: 2, add: ["text-3xl"] },
        { op: "set_classes", node: node(reactProject, home, "<main"), rev: 2, add: ["p-8"] },
        { op: "set_classes", node: node(reactProject, home, "<ul"), rev: 2, add: ["grid"] },
      ]),
    );
    const content = out[2].files![home];
    expect(content).toContain('<h2 className={cn("text-3xl", open && "underline")}>');
    expect(content).toContain('<main className="p-8">');
    expect(content).toContain('<ul className="grid">');
  });

  test("set_attr maps html names and replace_image drops srcSet", async () => {
    const out = await steps(
      request(
        reactProject,
        [
          { op: "replace_image", node: node(reactProject, hero, "<img"), rev: 1, media_id: "m1", alt: "Foto" },
          { op: "set_attr", node: node(reactProject, hero, "<h1"), rev: 1, name: "tabindex", value: "0" },
        ],
        { media: { m1: "https://cdn.example/m1.webp" } },
      ),
    );
    const content = out[1].files![hero];
    expect(content).toContain('<img src="https://cdn.example/m1.webp" alt="Foto" />');
    expect(content).toContain('<h1 className="text-4xl" tabIndex="0">');
  });

  test("a jsx copy into another file refuses, naming them, the identifiers the target file does not have", async () => {
    const pages: Project = {
      ...reactProject,
      files: {
        ...reactProject.files,
        "src/pages/Sobre.jsx": { content: 'import { Star } from "lucide-react";\nconst planos = [{ nome: "Básico" }];\nexport default function Sobre({ titulo }) {\n  return (\n    <main>\n      <ul>{planos.map((plano) => <li key={plano.nome}><Star className="size-4" />{plano.nome} {titulo}</li>)}</ul>\n      <p className="text-sm">{Math.round(2.4)} anos</p>\n    </main>\n  );\n}\n', rev: 4 },
      },
    };
    const list = node(pages, "src/pages/Sobre.jsx", "<ul>");
    const refused = await execute(request(pages, [{ op: "insert_section", at: { page: home }, rev: 2, source: list, source_rev: 4 }]));
    expect(refused).toEqual({ failures: [{ index: 0, code: "copy_reference_missing", field: "source", detail: "Star, planos, titulo" }] });
    const plain = await final(request(pages, [{ op: "insert_section", at: { page: home }, rev: 2, source: node(pages, "src/pages/Sobre.jsx", "<p "), source_rev: 4 }]), home);
    expect(plain).toContain("{Math.round(2.4)} anos</p>");
    const inside = await final(request(pages, [{ op: "insert_section", at: { after: list }, rev: 4, source: list, source_rev: 4 }]), "src/pages/Sobre.jsx");
    expect(inside.match(/planos\.map/g)).toHaveLength(2);
  });

  test("a jsx copy may use what the target file imports, declares or receives where it lands", async () => {
    const pages: Project = {
      ...reactProject,
      files: {
        ...reactProject.files,
        "src/pages/Sobre.jsx": { content: 'export default function Sobre() {\n  return (\n    <main>\n      <Hero title={items[0].title} />\n    </main>\n  );\n}\n', rev: 4 },
      },
    };
    const content = await final(request(pages, [{ op: "insert_section", at: { into: node(reactProject, home, "<main") }, rev: 2, source: node(pages, "src/pages/Sobre.jsx", "<Hero"), source_rev: 4 }]), home);
    expect(content).toContain("<Hero title={items[0].title} />");
  });

  test("an icon component instance takes class edits on its literal className and refuses a computed one", async () => {
    const icons: Project = { ...reactProject, files: { ...reactProject.files, "src/pages/Home.jsx": { content: 'import { Stethoscope } from "lucide-react";\nexport default function Home() {\n  return (\n    <main>\n      <span className="grid size-10"><Stethoscope className="size-6 text-brand" /></span>\n      <Stethoscope className={size} />\n    </main>\n  );\n}\n', rev: 2 } } };
    const icon = node(icons, home, "<Stethoscope className=\"size");
    const content = await final(request(icons, [{ op: "set_classes", node: icon, rev: 2, add: ["size-8"], remove: ["size-6"] }]), home);
    expect(content).toContain('<Stethoscope className="text-brand size-8" />');
    const computed = await execute(request(icons, [{ op: "set_classes", node: node(icons, home, "<Stethoscope className={"), rev: 2, add: ["size-8"] }]));
    expect(computed).toEqual({ failures: [{ index: 0, code: "node_invalid", field: "node" }] });
  });

  test("insert_section copies html markup into a react page as jsx", async () => {
    const mixed: Project = { ...reactProject, files: { ...reactProject.files, "src/pages/Sobre.jsx": { content: 'export default function Sobre() {\n  return (\n    <main>\n      <label htmlFor="x" className="block">Nome</label>\n    </main>\n  );\n}\n', rev: 4 } } };
    const content = await final(request(mixed, [{ op: "insert_section", at: { page: home }, rev: 2, source: node(mixed, "src/pages/Sobre.jsx", "<label"), source_rev: 4 }]), home);
    expect(content).toContain('<label htmlFor="x" className="block">Nome</label>');
  });

  test("insert_section converts html and remove refuses the returned root", async () => {
    const content = await final(request(reactProject, [{ op: "insert_section", at: { page: home }, rev: 2, html: '<section class="py-8"><label for="x">Nome</label></section>' }]), home);
    expect(content).toContain('      <section className="py-8"><label htmlFor="x">Nome</label></section>\n    </main>');
    const result = await execute(request(reactProject, [{ op: "remove", node: node(reactProject, home, "<main"), rev: 2 }]));
    expect(result).toEqual({ failures: [{ index: 0, code: "position_invalid", field: "node" }] });
  });

  test("duplicate inside jsx children", async () => {
    const content = await final(request(reactProject, [{ op: "duplicate", node: node(reactProject, home, "<h2"), rev: 2 }]), home);
    expect(content.split('<h2 className={cn("text-2xl", open && "underline")}>Serviços</h2>').length).toBe(3);
  });

  test("set_token targets the react stylesheet", async () => {
    const content = await final(request(reactProject, [{ op: "set_token", name: "--color-brand", value: "#111111" }]), "src/index.css");
    expect(content).toContain("--color-brand: #111111;");
  });
});

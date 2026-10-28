import { describe, expect, test } from "bun:test";
import { execute, extract } from "../src/execute/executor";
import { runJob } from "../src/jobs";
import { decodeExtract, RequestInvalid } from "../src/protocol";
import type { Project } from "../src/project";

const htmlProject: Project = {
  stack: "html",
  files: {
    "site.json": { content: '{"name":"Clinica","routes":[{"path":"/","page":"pages/index.html"}]}', rev: 1 },
    "pages/index.html": {
      content: '<main>\n  <section class="cta py-20">\n    <h2>Agende</h2>\n    <a href="#contato" data-vz-cta>Quero</a>\n  </section>\n</main>\n',
      rev: 3,
    },
  },
};

const reactProject: Project = {
  stack: "react",
  files: {
    "site.json": { content: '{"name":"Clinica","routes":[{"path":"/","page":"src/pages/Home.jsx"}]}', rev: 1 },
    "src/pages/Home.jsx": {
      content: 'export default function Home() {\n  return (\n    <main className="p-4">\n      <section className="cta py-20">\n        <h2 className="text-2xl">Agende</h2>\n      </section>\n    </main>\n  );\n}\n',
      rev: 2,
    },
  },
};

function node(project: Project, path: string, needle: string): string {
  const at = project.files[path].content.indexOf(needle);
  if (at < 0) throw new Error(`missing ${needle}`);
  return `${path}#${at}`;
}

describe("extract", () => {
  test("returns the exact source of an html element", () => {
    const out = extract({ project: htmlProject, node: node(htmlProject, "pages/index.html", "<section"), rev: 3, bases: {} });
    expect(out).toEqual({ source: '<section class="cta py-20">\n    <h2>Agende</h2>\n    <a href="#contato" data-vz-cta>Quero</a>\n  </section>' });
  });

  test("returns the exact source of a jsx element", () => {
    const out = extract({ project: reactProject, node: node(reactProject, "src/pages/Home.jsx", "<section"), rev: 2, bases: {} });
    expect(out).toEqual({ source: '<section className="cta py-20">\n        <h2 className="text-2xl">Agende</h2>\n      </section>' });
  });

  test("follows a stale revision through its base", () => {
    const base = '<main>\n<section class="cta py-20">\n    <h2>Agende</h2>\n    <a href="#contato" data-vz-cta>Quero</a>\n  </section>\n</main>\n';
    const out = extract({ project: htmlProject, node: `pages/index.html#${base.indexOf("<section")}`, rev: 2, bases: { "pages/index.html": [{ rev: 2, content: base }] } });
    expect(out).toEqual({ source: '<section class="cta py-20">\n    <h2>Agende</h2>\n    <a href="#contato" data-vz-cta>Quero</a>\n  </section>' });
  });

  test("refuses a node that is not an element", () => {
    expect(extract({ project: htmlProject, node: "pages/index.html#2", rev: 3, bases: {} })).toEqual({ failure: { code: "node_stale", field: "node" } });
  });

  test("refuses a node outside markup files", () => {
    expect(extract({ project: htmlProject, node: "site.json#0", rev: 1, bases: {} })).toEqual({ failure: { code: "node_invalid", field: "node" } });
  });

  test("refuses an unknown revision without a base", () => {
    expect(extract({ project: htmlProject, node: node(htmlProject, "pages/index.html", "<section"), rev: 1, bases: {} })).toEqual({ failure: { code: "node_stale", field: "node" } });
  });

  test("refuses a ref, which only exists inside a batch", () => {
    expect(extract({ project: htmlProject, node: "@novo", rev: 3, bases: {} })).toEqual({ failure: { code: "ref_unknown", field: "node" } });
  });
});

describe("extract protocol", () => {
  test("decodes a request with a node and a revision", () => {
    const decoded = decodeExtract({ project: htmlProject, node: "pages/index.html#7", rev: 3 });
    expect(decoded.node).toBe("pages/index.html#7");
    expect(decoded.rev).toBe(3);
    expect(decoded.bases).toEqual({});
  });

  test("refuses unknown fields and a missing node", () => {
    expect(() => decodeExtract({ project: htmlProject, node: "pages/index.html#7", rev: 3, extra: 1 })).toThrow(RequestInvalid);
    expect(() => decodeExtract({ project: htmlProject, rev: 3 })).toThrow(RequestInvalid);
  });

  test("runs as a job and answers with the source", async () => {
    const reply = await runJob("extract", JSON.stringify({ project: reactProject, node: node(reactProject, "src/pages/Home.jsx", "<h2"), rev: 2 }));
    expect(reply.status).toBe(200);
    expect(JSON.parse(reply.body)).toEqual({ source: '<h2 className="text-2xl">Agende</h2>' });
  });
});

describe("jsx sections", () => {
  const page = "src/pages/Home.jsx";

  test("insert verbatim into a react page", async () => {
    const snippet = '<section className="faixa">\n  <p className="text-lg">{"Oi"}</p>\n</section>';
    const result = await execute({
      project: reactProject,
      operations: [{ op: "insert_section", at: { after: node(reactProject, page, "<section") }, rev: 2, library_key: "snippet-faixa" }],
      texts: {},
      media: {},
      sections: {},
      jsx_sections: { "snippet-faixa": snippet },
      bases: {},
    });
    if (!("steps" in result)) throw new Error(JSON.stringify(result));
    const content = result.steps[0].files![page];
    expect(content).toContain('<section className="faixa">');
    expect(content).toContain('<p className="text-lg">{"Oi"}</p>');
  });

  test("refuse a jsx section in an html page", async () => {
    const result = await execute({
      project: htmlProject,
      operations: [{ op: "insert_section", at: { page: "pages/index.html" }, rev: 3, library_key: "snippet-faixa" }],
      texts: {},
      media: {},
      sections: {},
      jsx_sections: { "snippet-faixa": "<section />" },
      bases: {},
    });
    expect(result).toEqual({ failures: [{ index: 0, code: "library_key_invalid", field: "library_key" }] });
  });

  test("refuse jsx that does not parse", async () => {
    const result = await execute({
      project: reactProject,
      operations: [{ op: "insert_section", at: { after: node(reactProject, page, "<section") }, rev: 2, library_key: "snippet-faixa" }],
      texts: {},
      media: {},
      sections: {},
      jsx_sections: { "snippet-faixa": "<section><p></section>" },
      bases: {},
    });
    expect(result).toEqual({ failures: [{ index: 0, code: "library_key_invalid", field: "library_key" }] });
  });
});

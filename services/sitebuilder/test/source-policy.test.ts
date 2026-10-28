import { describe, expect, test } from "bun:test";
import { mkdtempSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { build, type BuildOutput, type BuildRequest } from "../src/build";
import { importDynamicForbidden, importForbidden, sourcePolicy } from "../src/build/analyze";
import { builderRoot, bundle } from "../src/build/bundle";
import { loadVendor } from "../src/build/vendor";
import type { Project } from "../src/project";
import { htmlProject, reactProject, runtime, shell } from "./fixtures/projects";

function request(project: Project, mode: BuildRequest["mode"] = "preview"): BuildRequest {
  return { mode, project, shell: { ...shell, integrations: {} }, runtime };
}

function editProject(project: Project, path: string, content: string): Project {
  return { ...project, files: { ...project.files, [path]: { content, rev: 1 } } };
}

const allowEverything = () => null;
const page = (body: string) => `${body}\nexport default function O() { return <p>ok</p>; }\n`;

function codes(content: string, path = "src/pages/Obrigado.jsx", refuse: (specifier: string) => string | null = allowEverything): string[] {
  return sourcePolicy(content, path, refuse).refusals.map((r) => r.code);
}

describe("source policy", () => {
  const dynamic: Array<[string, string]> = [
    ["template dynamic import", "const n = 'a'; import(`./${n}.json`);"],
    ["template literal without expressions", "import(`./a.js`);"],
    ["concatenated dynamic import", "const n = 'a'; import('./' + n);"],
    ["dynamic import with attributes", "import('./a.json', { with: { type: 'text' } });"],
    ["template dynamic import with a text attribute", "const n = 'a'; import(`./${n}.json`, { with: { type: 'text' } });"],
    ["static import with attributes", "import data from './a.json' with { type: 'json' };"],
    ["static import with a text attribute", "import data from './a.txt' with { type: 'text' };"],
    ["re export with attributes", "export * from './a.json' with { type: 'json' };"],
    ["named re export with attributes", "export { a } from './a.json' with { type: 'json' };"],
    ["require with a variable", "const n = 'a'; require(n);"],
    ["require with a template", "const n = 'a'; require(`./${n}`);"],
    ["require with two arguments", "require('./a', 1);"],
    ["require without arguments", "require();"],
    ["require.resolve", "require.resolve('./a');"],
    ["require.context", "require.context('./', true);"],
    ["require passed around", "const r = require; r('./a');"],
    ["import.meta.glob", "import.meta.glob('./*.js');"],
    ["import.meta.url in new URL", "new URL('./a.png', import.meta.url);"],
    ["any import.meta", "console.log(import.meta.env);"],
    ["new Worker", "new Worker('./w.js');"],
    ["new SharedWorker", "new SharedWorker('./w.js');"],
    ["service worker registration", "navigator.serviceWorker.register('./sw.js');"],
  ];
  for (const [name, source] of dynamic) {
    test(`refuses ${name}`, () => {
      expect(codes(page(source))).toContain(importDynamicForbidden);
    });
  }

  test("refuses import attributes written with assert", () => {
    const result = sourcePolicy(page("import data from './a.json' assert { type: 'json' };"), "src/pages/Obrigado.jsx", allowEverything);
    expect(result.refusals.length).toBeGreaterThan(0);
  });

  test("accepts literal static, dynamic and require imports and records them", () => {
    const result = sourcePolicy(
      page("import { motion } from 'framer-motion';\nexport * from './b.js';\nconst a = import('./a.js');\nconst c = require('./c.js');\nconst obj = { require: 1, meta: 2 };\nobj.require;\nclass K { require() {} }"),
      "src/pages/Obrigado.jsx",
      allowEverything,
    );
    expect(result.refusals).toEqual([]);
    expect(result.specifiers).toEqual(["framer-motion", "./b.js", "./a.js", "./c.js"]);
  });

  test("accepts typescript sources and records import equals requires", () => {
    const result = sourcePolicy("import x = require('./x');\ntype T = { require: string };\nexport const y: number = 1;\n", "src/lib/y.ts", allowEverything);
    expect(result.refusals).toEqual([]);
    expect(result.specifiers).toEqual(["./x"]);
  });

  test("refuses specifiers the caller refuses with the import refusal code", () => {
    const refuse = (specifier: string) => (specifier === "axios" ? "import axios is not in the allowed library list" : null);
    const result = sourcePolicy(page("import axios from 'axios';\nconst m = import('axios');\nconst r = require('axios');"), "src/pages/Obrigado.jsx", refuse);
    expect(result.refusals.map((r) => [r.code, r.message, r.line])).toEqual([
      [importForbidden, "import axios is not in the allowed library list", 1],
      [importForbidden, "import axios is not in the allowed library list", 2],
      [importForbidden, "import axios is not in the allowed library list", 3],
    ]);
  });

  test("reports where the refused import is", () => {
    const [refusal] = sourcePolicy("const n = 1;\n  import(`./${n}`);\n", "scripts/a.js", allowEverything).refusals;
    expect(refusal).toMatchObject({ line: 2, column: 3, code: importDynamicForbidden });
  });

  test("an unparsable module is refused", () => {
    const result = sourcePolicy("export default function O() { return <p>; }\n", "src/pages/Obrigado.jsx", allowEverything);
    expect(result.refusals).toHaveLength(1);
    expect(result.refusals[0].line).toBe(1);
  });
});

function firstError(out: BuildOutput) {
  expect(out.assets).toEqual([]);
  expect(out.routes).toEqual([]);
  expect(out.errors.length).toBeGreaterThan(0);
  return out.errors[0];
}

describe("builds refuse imports the bundler would expand", () => {
  test("a dynamic template import fails a react build", async () => {
    const out = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", page("const name = 'package';\nimport(`../../${name}.json`).then(console.log);"))));
    expect(firstError(out)).toMatchObject({ path: "src/pages/Obrigado.jsx", line: 2, code: importDynamicForbidden });
  });

  test("an import with a text attribute fails a react build", async () => {
    const out = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", page("const name = 'package';\nimport(`./${name}.json`, { with: { type: 'text' } });"))));
    expect(firstError(out)).toMatchObject({ path: "src/pages/Obrigado.jsx", code: importDynamicForbidden });
    const staticAttr = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", page("import raw from './Obrigado.jsx' with { type: 'text' };\nconsole.log(raw);"))));
    expect(firstError(staticAttr)).toMatchObject({ path: "src/pages/Obrigado.jsx", code: importDynamicForbidden });
  });

  test("a require with a non literal argument fails both stacks", async () => {
    const react = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", page("const name = 'package';\nrequire('../../' + name + '.json');"))));
    expect(firstError(react)).toMatchObject({ path: "src/pages/Obrigado.jsx", code: importDynamicForbidden });
    const html = await build(request(editProject(htmlProject, "scripts/motion.js", "const name = 'package';\nrequire(`../${name}.json`);\n")));
    expect(firstError(html)).toMatchObject({ path: "scripts/motion.js", code: importDynamicForbidden });
    const htmlDynamic = await build(request(editProject(htmlProject, "scripts/motion.js", "const name = 'package';\nimport(`../${name}.json`);\n")));
    expect(firstError(htmlDynamic)).toMatchObject({ path: "scripts/motion.js", code: importDynamicForbidden });
  });

  test("library refusals carry the import refusal code and keep the import list", async () => {
    const out = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", page("import axios from 'axios';\nconsole.log(axios);"))));
    expect(firstError(out)).toMatchObject({ path: "src/pages/Obrigado.jsx", code: importForbidden, message: "import axios is not in the allowed library list" });
    expect(out.imports).toContainEqual({ importer: "src/pages/Obrigado.jsx", specifier: "axios" });
  });

  test("a literal dynamic import of a project file still builds", async () => {
    const out = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", page("const lazy = () => import('../components/Hero.jsx');\nconsole.log(lazy);")), "publish"));
    expect(out.errors).toEqual([]);
    expect(out.routes.length).toBe(2);
  });

  test("the normal fixtures build without codes on success", async () => {
    for (const project of [reactProject, htmlProject]) {
      for (const mode of ["preview", "publish"] as const) {
        const out = await build(request(project, mode));
        expect(out.errors).toEqual([]);
      }
    }
  });
});

describe("bundle isolation", () => {
  const vendor = loadVendor();
  const contents = Object.fromEntries(Object.entries(reactProject.files).map(([path, file]) => [path, file.content]));

  async function withTempParent<T>(run: (parent: string) => Promise<T>): Promise<T> {
    const parent = mkdtempSync(join(tmpdir(), "vz-iso-"));
    writeFileSync(join(parent, "package.json"), '{"secret":"sibling-marker"}');
    const saved = { TMPDIR: process.env.TMPDIR, TMP: process.env.TMP, TEMP: process.env.TEMP };
    process.env.TMPDIR = process.env.TMP = process.env.TEMP = parent;
    try {
      return await run(parent);
    } finally {
      for (const [key, value] of Object.entries(saved)) {
        if (value === undefined) delete process.env[key];
        else process.env[key] = value;
      }
      rmSync(parent, { recursive: true, force: true });
    }
  }

  for (const [name, code] of [
    ["the builder folder", "const n = 'package'; import(`./${n}.json`).then(console.log);"],
    ["a parent folder", "const n = 'package'; import(`../${n}.json`).then(console.log);"],
    ["a parent folder through require", "const n = 'package'; require('../' + n + '.json');"],
    ["a text attribute", "const n = 'package'; import(`../${n}.json`, { with: { type: 'text' } });"],
  ] as const) {
    test(`an expansion that skips the policy cannot read ${name}`, async () => {
      await withTempParent(async () => {
        const out = await bundle({ stack: "react", files: contents, entries: [{ name: "probe", code }], preview: false, minify: false, vendor });
        const leaked = out.files.map((file) => new TextDecoder().decode(file.contents)).join("\n");
        expect(leaked).not.toContain("vozko-sitebuilder");
        expect(leaked).not.toContain("sibling-marker");
        expect(leaked).not.toContain("dependencies");
      });
    });
  }

  test("an expansion that escapes the job folder fails without a server path", async () => {
    await withTempParent(async (parent) => {
      const out = await bundle({ stack: "react", files: {}, entries: [{ name: "probe", code: "const n = 'package'; import(`../${n}.json`);" }], preview: false, minify: false, vendor });
      expect(out.files).toEqual([]);
      expect(out.errors.length).toBeGreaterThan(0);
      for (const error of out.errors) {
        expect(error.path).toBe("");
        expect(leaksServerPath(error.message)).toBe(false);
        expect(error.message).not.toContain(parent);
      }
    });
  });

  test("asset names stay the same from one job folder to the next", async () => {
    const names = async () => (await build(request(reactProject, "publish"))).assets.map((asset) => asset.path).sort();
    expect(await names()).toEqual(await names());
  });

  test("the job folder is removed after the bundle", async () => {
    await withTempParent(async (parent) => {
      await bundle({ stack: "react", files: contents, entries: [{ name: "probe", code: "console.log(1);" }], preview: false, minify: false, vendor });
      expect(readdirSync(parent)).toEqual(["package.json"]);
    });
  });
});

const absolute = /(^|[\s"'`(=])(?:[A-Za-z]:[\\/]|\/(?:[A-Za-z0-9_.-]+\/)+)/;

function leaksServerPath(text: string): boolean {
  const roots = [builderRoot, tmpdir(), process.cwd()].flatMap((root) => [root, root.replaceAll("\\", "/")]);
  return roots.some((root) => text.includes(root)) || absolute.test(text) || text.includes("node_modules");
}

describe("build errors never show server paths", () => {
  const hostile: Array<[string, Project]> = [
    ["template import", editProject(reactProject, "src/pages/Obrigado.jsx", page("const n = 'x';\nimport(`../../../../../../etc/${n}`);"))],
    ["escape import", editProject(reactProject, "src/pages/Obrigado.jsx", page("import x from '../../../../../etc/passwd';\nconsole.log(x);"))],
    ["absolute import", editProject(reactProject, "src/pages/Obrigado.jsx", page("import x from '/etc/passwd';\nconsole.log(x);"))],
    ["windows import", editProject(reactProject, "src/pages/Obrigado.jsx", page("import x from 'C:\\\\Windows\\\\win.ini';\nconsole.log(x);"))],
    ["deep package path", editProject(reactProject, "src/pages/Obrigado.jsx", page("import x from 'three/examples/jsm/nothing/here.js';\nconsole.log(x);"))],
    ["inline package subpath", editProject(reactProject, "src/pages/Obrigado.jsx", page("import x from 'react-dom/server.node';\nconsole.log(x);"))],
    ["syntax error", editProject(reactProject, "src/pages/Obrigado.jsx", "export default function O() { return <p>; }\n")],
    ["html template import", editProject(htmlProject, "scripts/motion.js", "const n = 'x';\nimport(`../../../${n}`);\n")],
    ["html missing module", editProject(htmlProject, "scripts/motion.js", "import x from './nothing.js';\nconsole.log(x);\n")],
  ];
  for (const [name, project] of hostile) {
    test(name, async () => {
      const out = await build(request(project));
      expect(out.errors.length).toBeGreaterThan(0);
      for (const error of out.errors) {
        expect(error.path === "" || project.files[error.path] !== undefined).toBe(true);
        expect(leaksServerPath(error.message)).toBe(false);
      }
    });
  }
});

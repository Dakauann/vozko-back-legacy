import { describe, expect, test } from "bun:test";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";
import { build, previewOnlyFiles, type BuildOutput, type BuildRequest } from "../src/build";
import { normalizeDocument } from "../src/build/shell";
import type { Project } from "../src/project";
import { htmlProject, reactProject, runtime, shell } from "./fixtures/projects";
import { prerenderAll } from "./fixtures/prerender";

const decoder = new TextDecoder();
const goldenDir = new URL("./golden/", import.meta.url);

function matchesGolden(name: string, html: string) {
  const file = new URL(name, goldenDir);
  if (process.env.UPDATE_GOLDEN === "1" || !existsSync(file)) {
    mkdirSync(goldenDir, { recursive: true });
    writeFileSync(file, html);
  }
  expect(html).toBe(readFileSync(file, "utf8"));
}

function text(out: BuildOutput, path: string): string {
  const asset = out.assets.find((candidate) => candidate.path === path);
  if (!asset) throw new Error(`missing asset ${path}: ${out.assets.map((a) => a.path).join(", ")}`);
  return decoder.decode(asset.body);
}

function golden(html: string): string {
  return html.replace(/-[A-Z0-9]{8}\.js/g, "-HASH.js").replace(/-[a-f0-9]{10}\.(js|css)/g, "-HASH.$1");
}

function request(project: Project, mode: BuildRequest["mode"]): BuildRequest {
  return { mode, project, shell, runtime };
}

function editProject(project: Project, path: string, content: string): Project {
  return { ...project, files: { ...project.files, [path]: { content, rev: 1 } } };
}

describe("editor capture library", () => {
  test("preview ships the screenshot library hashed and the bridge imports it by that name", async () => {
    const out = await build(request(reactProject, "preview"));
    const library = out.assets.find((a) => /assets\/vz-screenshot-[^/]+\.js$/.test(a.path));
    expect(library).toBeDefined();
    expect(decoder.decode(library!.body)).toBe(runtime["vz-screenshot.js"]);
    const bridge = out.assets.find((a) => /assets\/vz-editor-bridge-[^/]+\.js$/.test(a.path))!;
    const source = decoder.decode(bridge.body);
    expect(source).not.toContain('"./vz-screenshot.js"');
    expect(source).toContain(`"./${library!.path.split("/").pop()}"`);
  });

  test("publish never ships the screenshot library", async () => {
    const out = await build(request(reactProject, "publish"));
    expect(out.assets.some((a) => a.path.includes("vz-screenshot"))).toBe(false);
  });

  test("a preview without the screenshot library is refused", async () => {
    const { ["vz-screenshot.js"]: _dropped, ...partial } = runtime;
    await expect(build({ mode: "preview", project: reactProject, shell, runtime: partial })).rejects.toThrow("the screenshot library is required for previews");
  });
});

describe("runtime minification", () => {
  test("our runtime files ship minified with their exports intact while vendored libraries stay byte identical", async () => {
    for (const mode of ["preview", "publish"] as const) {
      const out = await build(request(htmlProject, mode));
      const served = (stem: string) => out.assets.find((a) => new RegExp(`assets/${stem}-[a-f0-9]{10}\.js$`).test(a.path));
      const runtimeAsset = served("vz-runtime")!;
      const source = decoder.decode(runtimeAsset.body);
      expect(source.length).toBeLessThan(runtime["vz-runtime.js"].length * 0.8);
      expect(source).not.toContain("attachWebchat");
      expect(source).not.toContain("MIN_FILL_MS");
      const pow = served("pow")!;
      expect(source).toContain(`./${pow.path.split("/").pop()}`);
      expect(decoder.decode(pow.body).length).toBeLessThan(runtime["pow.js"].length);
      const dir = mkdtempSync(join(tmpdir(), "vz-min-"));
      try {
        writeFileSync(join(dir, pow.path.split("/").pop()!), pow.body);
        writeFileSync(join(dir, "runtime.mjs"), source);
        const mod = await import(pathToFileURL(join(dir, "runtime.mjs")).href);
        expect(mod.TRAP_NAME).toBe("vz_website");
        expect(mod.messagesFor("en").bookingPreview).toBe("Preview: sample times, nothing is booked.");
        expect(mod.webchatLoader("https://api.vozko.test", "pk_1")).toBe("https://api.vozko.test/public/webchat/loader.js");
        expect(mod.formFields([["vz_website", "x"], ["lead.name", "Ana"]])).toEqual({ "lead.name": "Ana" });
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
      if (mode === "preview") {
        const bridge = decoder.decode(served("vz-editor-bridge")!.body);
        expect(bridge.length).toBeLessThan(runtime["vz-editor-bridge.js"].length * 0.8);
        expect(decoder.decode(served("vz-screenshot")!.body)).toBe(runtime["vz-screenshot.js"]);
        expect(decoder.decode(served("vz-tailwind-browser")!.body)).toBe(runtime["vz-tailwind-browser.js"]);
      }
    }
  });
});

describe("live class compiler", () => {
  for (const project of [reactProject, htmlProject]) {
    test(`a ${project.stack} preview loads the browser class compiler byte identical after the compiled stylesheet`, async () => {
      const out = await build(request(project, "preview"));
      const compiler = out.assets.find((a) => /assets\/vz-tailwind-browser-[a-f0-9]{10}\.js$/.test(a.path));
      expect(compiler).toBeDefined();
      expect(decoder.decode(compiler!.body)).toBe(runtime["vz-tailwind-browser.js"]);
      const index = text(out, "index.html");
      const tag = `<script defer="" src="/${compiler!.path}" data-vz-tailwind-browser=""></script>`;
      expect(index).toContain(tag);
      expect(index.indexOf(tag)).toBeGreaterThan(index.indexOf('rel="stylesheet"'));
      const source = index.match(/<style type="text\/tailwindcss">([\s\S]*?)<\/style>/);
      expect(source).not.toBeNull();
      expect(source![1]).toContain('@import "tailwindcss/theme" layer(theme);@import "tailwindcss/utilities" layer(utilities);');
      expect(source![1]).not.toContain("preflight");
      if (project === htmlProject) expect(source![1]).toContain("--color-brand: oklch(0.6 0.2 250);");
    });
  }

  test("a project theme cannot close the class compiler style", async () => {
    const hostile = { ...htmlProject, files: { ...htmlProject.files, "theme.css": { content: '@import "tailwindcss";\n@theme {\n  --x: "</style><script>alert(1)</script>";\n}\n', rev: 1 } } };
    const index = text(await build(request(hostile, "preview")), "index.html");
    const source = index.match(/<style type="text\/tailwindcss">([\s\S]*?)<\/style>/)![1];
    expect(source).not.toContain("<");
  });

  test("publish never ships any preview only file even when it is handed one", async () => {
    for (const project of [reactProject, htmlProject]) {
      const out = await build(request(project, "publish"));
      for (const name of previewOnlyFiles) {
        const stem = name.replace(/\.js$/, "");
        expect(out.assets.some((a) => a.path.includes(stem))).toBe(false);
        for (const asset of out.assets) expect(decoder.decode(asset.body)).not.toContain("data-vz-tailwind-browser");
      }
    }
  });

  test("a preview without the browser class compiler is refused", async () => {
    const { ["vz-tailwind-browser.js"]: _dropped, ...partial } = runtime;
    await expect(build({ mode: "preview", project: reactProject, shell, runtime: partial })).rejects.toThrow("the class compiler is required for previews");
  });
});

describe("react build", () => {
  test("preview tags the bundle, adds the bridge and revisions, and skips integrations", async () => {
    const out = await build(request(reactProject, "preview"));
    expect(out.errors).toEqual([]);
    const index = text(out, "index.html");
    expect(golden(index)).toMatchSnapshot();
    expect(index).toContain('<meta name="vz-revs"');
    expect(index).toMatch(/<html[^>]*\sdata-vz-preview/);
    expect(index).toContain('<meta name="robots" content="noindex">');
    expect(index).toContain("vz-editor-bridge-");
    expect(index).not.toContain("data-vz-consent");
    expect(index).toContain('<div id="root"></div>');
    const entry = /src="\/(assets\/app-[^"]+\.js)"/.exec(index)![1];
    const js = text(out, entry) + out.assets.filter((a) => a.path.includes("/chunks/")).map((a) => decoder.decode(a.body)).join("");
    expect(js).toContain('"data-vz-node": "src/components/Hero.jsx#');
    expect(js).toContain('"data-vz-inst": "src/pages/Home.jsx#');
    expect(js).toContain('"data-vz-i": __vzI');
    expect(out.imports).toContainEqual({ importer: "src/pages/Home.jsx", specifier: "framer-motion" });
    expect(out.imports).toContainEqual({ importer: "src/pages/Home.jsx", specifier: "@/lib/utils" });
    expect(out.warnings.map((w) => [w.path, w.code])).toContainEqual(["src/pages/Home.jsx", "motion_hidden_text"]);
    expect(out.warnings.map((w) => [w.path, w.code])).toContainEqual(["src/pages/Home.jsx", "class_dynamic"]);
    expect(out.prerender_path).toBe("");
  });

  test("publish strips preview data, hashes assets, writes integrations and the prerender entry", async () => {
    const out = await build(request(reactProject, "publish"));
    expect(out.errors).toEqual([]);
    const index = text(out, "index.html");
    expect(golden(index)).toMatchSnapshot();
    expect(normalizeDocument(index)).toBe(index);
    expect(index).not.toContain("vz-revs");
    expect(index).not.toContain("data-vz-preview");
    expect(index).not.toContain("data-vz-node");
    expect(index).not.toContain("vz-editor-bridge");
    expect(index).toContain('<div id="root"><!--vz-prerender--></div>');
    expect(index).toContain('data-vz-consent="marketing" data-vz-integration="meta_pixel"');
    expect(index).toContain('data-vz-consent="analytics" data-vz-integration="ga4"');
    expect(index).toContain('<vz-consent text="Usamos cookies"></vz-consent>');
    expect(index).toContain('data-vz-forms="{&quot;contato&quot;:&quot;fk_contato&quot;}"');
    expect(index).not.toMatch(/<script>[^<]/);
    expect(out.routes).toEqual([
      { route: "/", html_path: "index.html", interactive: true },
      { route: "/obrigado", html_path: "obrigado/index.html", interactive: false },
    ]);
    expect(text(out, "obrigado/index.html")).toContain('<meta name="robots" content="noindex">');
    expect(out.prerender_path).toBe("__vz/prerender.html");
    expect(out.assets.some((a) => a.path.startsWith("assets/vendor/react-dom-client-"))).toBe(true);
    expect(out.assets.some((a) => a.path.startsWith("assets/vendor/lucide"))).toBe(false);
    const runtimeAsset = out.assets.find((a) => a.path.startsWith("assets/vz-runtime-"))!;
    const pow = out.assets.find((a) => a.path.startsWith("assets/pow-"))!;
    expect(decoder.decode(runtimeAsset.body)).toContain(`"./${pow.path.split("/").pop()}"`);
    const css = decoder.decode(out.assets.find((a) => a.path.startsWith("assets/index-") && a.path.endsWith(".css"))!.body);
    expect(css).toContain(".bg-brand");
    expect(css).toContain(".text-4xl");
  });

  test("the prerender entry renders the markup hydrateRoot expects", async () => {
    const out = await build(request(reactProject, "publish"));
    const rendered = await prerenderAll(out, ["/", "/obrigado"]);
    expect(rendered["/"]).toContain("Bem-vindo");
    expect(rendered["/"]).toContain('<div class="min-h-screen bg-white">');
    expect(rendered["/"]).toContain("Implante");
    expect(rendered["/obrigado"]).toContain("Obrigado!");
    await expect(prerenderAll(out, ["/nada"])).rejects.toThrow();
    matchesGolden("react-publish-index.html", text(out, "index.html").replace("<!--vz-prerender-->", rendered["/"]));
  });

  test("imports outside the allow list and missing files are build errors", async () => {
    const axios = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", 'import axios from "axios";\nexport default function O() { return <p>{String(axios)}</p>; }\n'), "preview"));
    expect(axios.assets).toEqual([]);
    expect(axios.errors[0]).toMatchObject({ path: "src/pages/Obrigado.jsx", message: "import axios is not in the allowed library list" });
    expect(axios.imports).toContainEqual({ importer: "src/pages/Obrigado.jsx", specifier: "axios" });
    const escape = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", 'import x from "../../site.json";\nexport default function O() { return <p>{x}</p>; }\n'), "preview"));
    expect(escape.errors[0].message).toContain("does not resolve to a project file");
    const syntax = await build(request(editProject(reactProject, "src/pages/Obrigado.jsx", "export default function O() { return <p>; }\n"), "preview"));
    expect(syntax.errors[0].path).toBe("src/pages/Obrigado.jsx");
    expect(syntax.errors[0].line).toBe(1);
  });
});

describe("html build", () => {
  test("preview tags pages and partials, bundles scripts and compiles the theme", async () => {
    const out = await build(request(htmlProject, "preview"));
    expect(out.errors).toEqual([]);
    const index = text(out, "index.html");
    expect(golden(index)).toMatchSnapshot();
    expect(index).toContain('<header class="p-4" data-vz-node="partials/header.html#0">');
    expect(index).toContain(`<section class="bg-brand py-20" data-vz-node="pages/index.html#${htmlProject.files["pages/index.html"].content.indexOf("<section")}">`);
    expect(index).toMatch(/<script type="module" src="\/assets\/script-motion-[A-Z0-9]+\.js" data-vz-node="pages\/index\.html#\d+"><\/script>/);
    expect(out.imports).toEqual([{ importer: "scripts/motion.js", specifier: "gsap" }]);
    expect(out.assets.some((a) => a.path.startsWith("assets/vendor/gsap-"))).toBe(true);
  });

  test("publish emits normal html without preview attributes", async () => {
    const out = await build(request(htmlProject, "publish"));
    expect(out.errors).toEqual([]);
    const index = text(out, "index.html");
    expect(golden(index)).toMatchSnapshot();
    expect(index).not.toContain("data-vz-node");
    expect(normalizeDocument(index)).toBe(index);
    expect(out.routes).toEqual([
      { route: "/", html_path: "index.html", interactive: true },
      { route: "/obrigado", html_path: "obrigado/index.html", interactive: false },
    ]);
  });

  test("broken includes and react imports are refused", async () => {
    const missing = await build(request(editProject(htmlProject, "pages/obrigado.html", '<vz-include src="partials/nada.html"></vz-include>'), "preview"));
    expect(missing.errors[0]).toMatchObject({ path: "pages/obrigado.html", message: "vz-include partials/nada.html does not point to a partial" });
    const loop = await build(request(editProject(htmlProject, "partials/header.html", '<vz-include src="partials/header.html"></vz-include>'), "preview"));
    expect(loop.errors[0].message).toContain("includes itself");
    const react = await build(request(editProject(htmlProject, "scripts/motion.js", 'import React from "react";\nconsole.log(React);\n'), "preview"));
    expect(react.errors[0].message).toBe("import react is not in the allowed library list");
  });

  test("invalid integrations refuse the build", async () => {
    await expect(build({ ...request(htmlProject, "publish"), shell: { ...shell, integrations: { ga4: "x\";alert(1)//" } } })).rejects.toThrow("integration ga4 has an invalid id");
  });
});

describe("publish site files and head", () => {
  test("html publish writes sitemap, robots, llms, canonical, favicon and structured data", async () => {
    const out = await build(request(htmlProject, "publish"));
    expect(out.errors).toEqual([]);
    expect(text(out, "sitemap.xml")).toBe(
      '<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n  <url><loc>https://sorriso.vozko.site/</loc></url>\n</urlset>\n',
    );
    expect(text(out, "robots.txt")).toBe("User-agent: *\nAllow: /\nSitemap: https://sorriso.vozko.site/sitemap.xml\n");
    expect(text(out, "llms.txt")).toBe("# Sorriso perfeito\n\n> Implantes e clareamento\n\n## Páginas\n\n- [Sorriso perfeito](https://sorriso.vozko.site/): Implantes e clareamento\n");
    expect(out.assets.find((a) => a.path === "sitemap.xml")!.content_type).toBe("application/xml; charset=utf-8");
    const thanks = text(out, "obrigado/index.html");
    expect(thanks).toContain('<link rel="canonical" href="https://sorriso.vozko.site/obrigado">');
    expect(thanks).toContain('<link rel="icon" href="https://cdn.vozko.test/favicon.png">');
    const slash = String.fromCharCode(92);
    expect(thanks).toContain(`<script type="application/ld+json">{"@context":"https://schema.org","@type":"Dentist","name":"Clínica ${slash}u003c/script${slash}u003e Sorriso"}</script>`);
    const index = text(out, "index.html");
    const consent = index.indexOf("vz-runtime-consent-");
    expect(consent).toBeGreaterThan(0);
    expect(consent).toBeLessThan(index.indexOf('data-vz-integration="ga4"'));
    matchesGolden("html-publish-index.html", index);
    matchesGolden("html-publish-obrigado.html", thanks);
  });

  test("robots disallows everything when every route is noindex, and the origin placeholder stands in", async () => {
    const pages = { "/": { noindex: true }, "/obrigado": { noindex: true } };
    const out = await build({ ...request(htmlProject, "publish"), shell: { ...shell, pages, primary_origin: undefined } });
    expect(text(out, "robots.txt")).toBe("User-agent: *\nDisallow: /\n");
    expect(text(out, "sitemap.xml")).not.toContain("<url>");
    expect(text(out, "index.html")).toContain('<link rel="canonical" href="__VZ_ORIGIN__/">');
  });

  test("the consent default is emitted only with integrations and site files only on publish", async () => {
    const plain = await build({ ...request(htmlProject, "publish"), shell: { ...shell, integrations: {} } });
    expect(plain.assets.some((a) => a.path.includes("vz-runtime-consent"))).toBe(false);
    expect(text(plain, "index.html")).not.toContain("<vz-consent");
    const preview = await build(request(htmlProject, "preview"));
    expect(preview.assets.some((a) => a.path.includes("vz-runtime-consent") || a.path === "sitemap.xml")).toBe(false);
  });
});

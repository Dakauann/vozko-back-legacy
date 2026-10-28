import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { build } from "../src/build";
import { execute } from "../src/execute/executor";
import type { Project } from "../src/project";
import { prerenderAll } from "./fixtures/prerender";
import { runtime, shell } from "./fixtures/projects";
import { starters } from "./fixtures/starters";

const sectionsRoot = fileURLToPath(new URL("../../../usecases/sitebuild/sections/", import.meta.url));

const sections = readdirSync(sectionsRoot)
  .filter((name) => name.endsWith(".html"))
  .map((name) => ({ key: name.slice(0, -".html".length), html: readFileSync(join(sectionsRoot, name), "utf8") }));

function homePage(project: Project): string {
  const manifest = JSON.parse(project.files["site.json"].content) as { routes: Array<{ path: string; page: string }> };
  return manifest.routes.find((route) => route.path === "/")!.page;
}

async function insert(project: Project, key: string, html: string): Promise<Project> {
  const page = homePage(project);
  const result = await execute({
    project,
    operations: [{ op: "insert_section", at: { page }, rev: project.files[page].rev, library_key: key }],
    texts: {},
    media: {},
    sections: { [key]: html },
    bases: {},
  });
  if (!("steps" in result)) throw new Error(JSON.stringify(result));
  const files = { ...project.files };
  for (const step of result.steps) {
    for (const [path, content] of Object.entries(step.files ?? {})) files[path] = { content, rev: (files[path]?.rev ?? 0) + 1 };
  }
  return { stack: project.stack, files };
}

function headingOf(html: string): string {
  return /<h[12][^>]*>([^<]+)</.exec(html)?.[1] ?? /<p class="font-display[^"]*">([^<]+)</.exec(html)![1];
}

describe("library sections insert and build in both stacks", () => {
  const bases = starters().filter(({ name }) => name.endsWith("/blank"));
  test("both blank starters exist", () => {
    expect(bases.map(({ name }) => name).sort()).toEqual(["html/blank", "react/blank"]);
  });
  for (const { key, html } of sections) {
    for (const base of bases) {
      test(`${key} into ${base.name}`, async () => {
        const project = await insert(base.project, key, html);
        const page = project.files[homePage(project)].content;
        if (project.stack === "react") {
          expect(page).toContain("className=");
          expect(page).not.toContain(' class="');
        }
        const out = await build({ mode: "publish", project, shell: { ...shell, integrations: {} }, runtime });
        expect(out.errors).toEqual([]);
        const heading = headingOf(html);
        if (project.stack === "react") {
          const rendered = await prerenderAll(out, ["/"]);
          expect(rendered["/"]).toContain(heading);
        } else {
          const home = out.assets.find((asset) => asset.path === out.routes.find((route) => route.route === "/")!.html_path)!;
          expect(new TextDecoder().decode(home.body)).toContain(heading);
        }
      });
    }
  }
});

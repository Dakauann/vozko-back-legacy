import { createHash } from "node:crypto";
import { writeFileSync } from "node:fs";
import { build } from "../src/build";
import type { ShellInput } from "../src/build/shell";
import { execute } from "../src/execute/executor";
import type { Operation } from "../src/execute/types";
import type { Project } from "../src/project";
import { prerenderAll } from "../test/fixtures/prerender";
import { runtime } from "../test/fixtures/projects";
import { starters } from "../test/fixtures/starters";

const [starterName, target] = process.argv.slice(2);
if (!starterName || !target) {
  console.error("usage: bun run scripts/record-build.ts <stack/starter> <output.json>");
  process.exit(1);
}
const starter = starters().find(({ name }) => name === starterName);
if (!starter) {
  console.error(`unknown starter ${starterName}`);
  process.exit(1);
}

const recordedFormKey = "fk_recorded_contato";
const recordedAPI = "https://api.recorded.test";
const shell: ShellInput = { api: recordedAPI, forms: { contato: recordedFormKey }, pages: {}, integrations: {} };
const maxBody = 4096;
const whatsapp = 'data-vz-whatsapp="phone-1" data-vz-text="Olá! Vim pela página."';
const edits: Operation[] = Object.entries(starter.project.files)
  .filter(([, file]) => file.content.includes('data-vz-whatsapp=""'))
  .map(([path]) => ({ op: "edit_file", path, old: 'data-vz-whatsapp=""', new: whatsapp, replace_all: true }));

async function edited(project: Project): Promise<Project> {
  const result = await execute({ project, operations: edits, texts: {}, media: {}, sections: {}, bases: {} });
  if (!("steps" in result)) throw new Error(JSON.stringify(result));
  const files = { ...project.files };
  for (const step of result.steps) {
    for (const [path, content] of Object.entries(step.files ?? {})) files[path] = { content, rev: files[path].rev + 1 };
  }
  return { stack: project.stack, files };
}

function encode(path: string, body: Uint8Array): { body: string; stub: boolean } {
  if (body.length > maxBody && /\/vendor\//.test(path)) return { body: Buffer.from(`/* recorded ${path} */`).toString("base64"), stub: true };
  return { body: Buffer.from(body).toString("base64"), stub: false };
}

const project = await edited(starter.project);
const sources = Object.fromEntries(Object.entries(project.files).map(([path, file]) => [path, createHash("sha256").update(file.content).digest("hex")]));
const recording: Record<string, unknown> = { starter: starterName, form_key: recordedFormKey, api: recordedAPI, edits, sources };
for (const mode of ["preview", "publish"] as const) {
  const out = await build({ mode, project, shell, runtime });
  if (out.errors.length > 0) throw new Error(JSON.stringify(out.errors));
  const rendered = mode === "publish" && out.prerender_path ? await prerenderAll(out, out.routes.map((r) => r.route)) : {};
  recording[mode] = {
    routes: out.routes,
    prerender_path: out.prerender_path,
    imports: out.imports,
    assets: out.assets.map((a) => ({ path: a.path, content_type: a.content_type, ...encode(a.path, a.body) })),
    rendered,
  };
}
writeFileSync(target, JSON.stringify(recording, null, 1) + "\n");
console.log(`recorded ${starterName} into ${target}`);
process.exit(0);

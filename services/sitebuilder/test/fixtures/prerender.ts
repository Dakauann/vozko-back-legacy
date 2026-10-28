import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { pathToFileURL } from "node:url";
import type { BuildOutput } from "../../src/build";

const shared = mkdtempSync(join(tmpdir(), "vz-prerender-"));
process.on("exit", () => rmSync(shared, { recursive: true, force: true }));
const base = pathToFileURL(shared).href + "/";
const entries = new Map<string, (route: string) => Promise<string>>();

export async function prerenderAll(out: BuildOutput, routes: string[]): Promise<Record<string, string>> {
  for (const asset of out.assets) {
    const target = join(shared, asset.path);
    mkdirSync(dirname(target), { recursive: true });
    const body = asset.path.endsWith(".js") ? new TextDecoder().decode(asset.body).replaceAll('"/assets/', '"' + base + "assets/") : asset.body;
    writeFileSync(target, body);
  }
  const page = new TextDecoder().decode(out.assets.find((a) => a.path === out.prerender_path)!.body);
  const entry = /src="[/]([^"]+)"/.exec(page)![1];
  const location = { pathname: "/" };
  const scope = globalThis as unknown as { window: unknown; document: unknown; __vzPrerender?: (route: string) => Promise<string> };
  scope.window = Object.assign(globalThis, { location, history: { replaceState: (_s: unknown, _t: string, url: string) => (location.pathname = url) } });
  scope.document = { getElementById: () => ({ hasChildNodes: () => false }) };
  if (!entries.has(entry)) {
    await import(pathToFileURL(join(shared, entry)).href);
    entries.set(entry, scope.__vzPrerender!);
  }
  const render = entries.get(entry)!;
  const rendered: Record<string, string> = {};
  for (const route of routes) rendered[route] = await render(route);
  return rendered;
}

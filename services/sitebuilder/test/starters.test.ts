import { describe, expect, test } from "bun:test";
import { build } from "../src/build";
import { runtime, shell } from "./fixtures/projects";
import { prerenderAll } from "./fixtures/prerender";
import { starters } from "./fixtures/starters";

describe("platform starters build", () => {
  for (const { name, project } of starters()) {
    for (const mode of ["preview", "publish"] as const) {
      test(`${name} ${mode}`, async () => {
        const out = await build({ mode, project, shell: { ...shell, integrations: {} }, runtime });
        expect(out.errors).toEqual([]);
        expect(out.routes.length).toBeGreaterThan(0);
        if (mode === "publish" && project.stack === "react") {
          const rendered = await prerenderAll(out, out.routes.map((r) => r.route));
          for (const route of out.routes) {
            expect(rendered[route.route].length).toBeGreaterThan(20);
            expect(rendered[route.route]).not.toContain("<script");
          }
          if (out.routes.length > 1) expect(rendered[out.routes[0].route]).not.toBe(rendered[out.routes[1].route]);
        }
      });
    }
  }
});

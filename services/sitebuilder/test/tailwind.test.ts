import { describe, expect, test } from "bun:test";
import { classValidator, compileCss, scanCandidates } from "../src/tailwind";

const styles = {
  "theme.css": '@import "tailwindcss";\n@import "./extra.css";\n@theme {\n  --color-brand: oklch(0.6 0.2 250);\n}\n',
  "extra.css": ".hero { color: red; }\n",
};

describe("tailwind", () => {
  test("compiles scanned candidates with theme tokens", async () => {
    const candidates = scanCandidates([{ content: '<div class="bg-brand py-20 hero">', extension: "html" }]);
    expect(candidates).toContain("bg-brand");
    const css = await compileCss("theme.css", styles, candidates);
    expect(css).toContain(".bg-brand");
    expect(css).toContain(".py-20");
    expect(css).toContain(".hero");
  });

  test("validates classes against the design system and project css", async () => {
    const valid = await classValidator("theme.css", styles);
    expect(valid("py-24")).toBe(true);
    expect(valid("md:py-24")).toBe(true);
    expect(valid("bg-brand")).toBe(true);
    expect(valid("hero")).toBe(true);
    expect(valid("not-a-class")).toBe(false);
    expect(valid("md:hero")).toBe(false);
  });

  test("plugins and outside stylesheets are refused", async () => {
    await expect(compileCss("a.css", { "a.css": '@import "tailwindcss";\n@plugin "x";' }, [])).rejects.toThrow();
    await expect(compileCss("a.css", { "a.css": '@import "../../etc/x.css";' }, [])).rejects.toThrow();
    await expect(compileCss("a.css", { "a.css": '@import "other-package";' }, [])).rejects.toThrow();
  });
});

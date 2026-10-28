import { describe, expect, test } from "bun:test";
import { setCssVariable, setThemeToken } from "../src/execute/theme";

describe("setThemeToken", () => {
  const css = '@import "tailwindcss";\n@theme {\n  --color-brand: oklch(0.6 0.2 250);\n  --hero-gap: 4rem;\n}\n';

  test("replaces an existing token value only", () => {
    expect(setThemeToken(css, "--color-brand", "oklch(0.7 0.1 30)").content).toBe(
      '@import "tailwindcss";\n@theme {\n  --color-brand: oklch(0.7 0.1 30);\n  --hero-gap: 4rem;\n}\n',
    );
  });

  test("does not confuse a longer token with a prefix", () => {
    const edited = setThemeToken('@theme {\n  --color-brand-dark: #000;\n}\n', "--color-brand", "#fff").content;
    expect(edited).toBe('@theme {\n  --color-brand-dark: #000;\n  --color-brand: #fff;\n}\n');
  });

  test("adds a missing token inside the block", () => {
    expect(setThemeToken(css, "--radius-card", "1rem").content).toBe(
      '@import "tailwindcss";\n@theme {\n  --color-brand: oklch(0.6 0.2 250);\n  --hero-gap: 4rem;\n  --radius-card: 1rem;\n}\n',
    );
  });

  test("creates the block after the tailwind import", () => {
    expect(setThemeToken('@import "tailwindcss";\n.a { color: red; }\n', "--color-brand", "#000").content).toBe(
      '@import "tailwindcss";\n@theme {\n  --color-brand: #000;\n}\n.a { color: red; }\n',
    );
  });

  test("ignores tokens inside comments", () => {
    const edited = setThemeToken('@theme {\n  /* --x: 1; */\n  --x: 2;\n}\n', "--x", "3").content;
    expect(edited).toBe('@theme {\n  /* --x: 1; */\n  --x: 3;\n}\n');
  });
});

describe("setCssVariable", () => {
  test("finds variables outside the theme block", () => {
    expect(setCssVariable(":root {\n  --hero-gap: 4rem;\n}\n", "--hero-gap", "2.5rem")?.content).toBe(":root {\n  --hero-gap: 2.5rem;\n}\n");
  });

  test("returns null when the variable is not declared", () => {
    expect(setCssVariable(":root {}\n", "--x", "1")).toBeNull();
  });
});

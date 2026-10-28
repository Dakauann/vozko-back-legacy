import { describe, expect, test } from "bun:test";
import { build, type BuildRequest } from "../src/build";
import { integrationCode, IntegrationRefused } from "../src/build/shell";
import { decodeBuild, RequestInvalid } from "../src/protocol";
import { htmlProject, runtime, shell } from "./fixtures/projects";

const loaders = {
  meta_pixel: "https://connect.facebook.net/en_US/fbevents.js",
  ga4: "https://www.googletagmanager.com/gtag/js",
  gtm: "https://www.googletagmanager.com/gtm.js",
  google_ads: "https://www.googletagmanager.com/gtag/js",
  tiktok_pixel: "https://analytics.tiktok.com/i18n/pixel/events.js",
  clarity: "https://www.clarity.ms/tag/",
};

describe("integration loaders come from the request", () => {
  test("every integration loads from the exact url it was given", () => {
    const ids: Record<string, string> = { meta_pixel: "1234567890", ga4: "G-ABC1234", gtm: "GTM-ABC123", google_ads: "AW-123456789", tiktok_pixel: "ABCDEFGHIJ12", clarity: "abc123def" };
    for (const [kind, id] of Object.entries(ids)) {
      const moved = "https://loader.example/" + kind + ".js";
      const { code } = integrationCode(kind, id, moved);
      expect(code).toContain(JSON.stringify(moved).slice(0, -1));
      const original = integrationCode(kind, id, loaders[kind as keyof typeof loaders]).code;
      expect(original).not.toContain("loader.example");
    }
  });

  test("an integration without a loader refuses", () => {
    expect(() => integrationCode("ga4", "G-ABC1234", undefined)).toThrow(IntegrationRefused);
    expect(() => integrationCode("ga4", "G-ABC1234", "")).toThrow("integration ga4 has no loader");
  });

  test("a publish build without the loader refuses", async () => {
    const request: BuildRequest = { mode: "publish", project: htmlProject, shell: { ...shell, loaders: {} }, runtime };
    await expect(build(request)).rejects.toThrow("integration ga4 has no loader");
  });

  test("the protocol accepts only exact https loader urls", () => {
    const body = (value: unknown) => ({ mode: "publish", project: { stack: "html", files: {} }, shell: { api: "https://api.example", loaders: value }, runtime: {} });
    expect(decodeBuild(body({ ga4: loaders.ga4 })).shell.loaders).toEqual({ ga4: loaders.ga4 });
    expect(decodeBuild(body(undefined)).shell.loaders).toEqual({});
    for (const bad of ["http://www.googletagmanager.com/gtag/js", "https://x.example/a.js?id=1", "https://x.example/a.js#x", 'https://x.example/"a', 7]) {
      expect(() => decodeBuild(body({ ga4: bad }))).toThrow(RequestInvalid);
    }
  });
});

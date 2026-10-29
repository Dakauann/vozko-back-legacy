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

type Listener = (event: { detail: unknown }) => void;

function sandbox(kind: string, id: string) {
  const listeners: Record<string, Listener[]> = {};
  const script = { parentNode: { insertBefore() {} } };
  const document = { createElement: () => ({ style: {} }), getElementsByTagName: () => [script], head: { appendChild() {} } };
  const window: Record<string, any> = { addEventListener: (type: string, listener: Listener) => (listeners[type] ??= []).push(listener) };
  window.window = window;
  window.document = document;
  const { code } = integrationCode(kind, id, loaders[kind as keyof typeof loaders]);
  const fbq = (...args: unknown[]) => window.fbq(...args);
  new Function("window", "document", "fbq", code)(window, document, fbq);
  const fire = (type: string, detail: unknown) => {
    for (const listener of listeners[type] ?? []) listener({ detail });
  };
  const pushed = () => (window.dataLayer ?? []).map((entry: unknown) => (entry && typeof entry === "object" && "length" in (entry as object) ? Array.from(entry as ArrayLike<unknown>) : entry));
  return { window, fire, pushed, listeners };
}

describe("conversion events follow the runtime lead and contact signals", () => {
  test("meta pixel tracks Lead with the server event id and Contact on WhatsApp", () => {
    const { window, fire } = sandbox("meta_pixel", "1234567890");
    fire("vz:lead", { eventId: "sub-1", form: "contato" });
    fire("vz:contact", { target: "https://wa.me/5511" });
    const queue = window.fbq.queue.map((args: ArrayLike<unknown>) => Array.from(args));
    expect(queue).toContainEqual(["track", "Lead", {}, { eventID: "sub-1" }]);
    expect(queue).toContainEqual(["track", "Contact"]);
  });

  test("ga4 sends generate_lead", () => {
    const { fire, pushed } = sandbox("ga4", "G-ABC1234");
    fire("vz:lead", { eventId: "sub-1", form: "contato" });
    expect(pushed()).toContainEqual(["event", "generate_lead", { form_id: "contato" }]);
  });

  test("google ads keeps the conversion label and fires the conversion on a lead", () => {
    const { fire, pushed } = sandbox("google_ads", "AW-123456789/AbC-dEf_1");
    expect(pushed()).toContainEqual(["config", "AW-123456789"]);
    fire("vz:lead", { eventId: "sub-1", form: "contato" });
    expect(pushed()).toContainEqual(["event", "conversion", { send_to: "AW-123456789/AbC-dEf_1", transaction_id: "sub-1" }]);
  });

  test("google ads without a label configures the tag but sends no conversion", () => {
    const { fire, pushed, listeners } = sandbox("google_ads", "AW-123456789");
    fire("vz:lead", { eventId: "sub-1", form: "contato" });
    expect(listeners["vz:lead"] ?? []).toHaveLength(0);
    expect(pushed().some((entry: unknown) => Array.isArray(entry) && entry[1] === "conversion")).toBe(false);
  });

  test("gtm receives dataLayer events for leads and contacts", () => {
    const { fire, pushed } = sandbox("gtm", "GTM-ABC123");
    fire("vz:lead", { eventId: "sub-1", form: "contato" });
    fire("vz:contact", { target: "https://wa.me/5511" });
    expect(pushed()).toContainEqual({ event: "generate_lead", vz_event_id: "sub-1", vz_form: "contato" });
    expect(pushed()).toContainEqual({ event: "contact", vz_target: "https://wa.me/5511" });
  });

  test("tiktok tracks SubmitForm with the event id and Contact", () => {
    const { window, fire } = sandbox("tiktok_pixel", "ABCDEFGHIJ12");
    fire("vz:lead", { eventId: "sub-1", form: "contato" });
    fire("vz:contact", { target: "https://wa.me/5511" });
    const calls = Array.from(window.ttq as unknown[][]);
    expect(calls).toContainEqual(["track", "SubmitForm", {}, { event_id: "sub-1" }]);
    expect(calls).toContainEqual(["track", "Contact"]);
  });

  test("a lead without an event id still tracks without one", () => {
    const { window, fire } = sandbox("meta_pixel", "1234567890");
    fire("vz:lead", { form: "contato" });
    expect(window.fbq.queue.map((args: ArrayLike<unknown>) => Array.from(args))).toContainEqual(["track", "Lead", {}, {}]);
  });
});

describe("site wide chat", () => {
  test("the protocol takes a widget reference and refuses anything else", () => {
    const body = (webchat: unknown) => ({ mode: "publish", project: { stack: "html", files: {} }, shell: { api: "https://api.example", webchat }, runtime: {} });
    expect(decodeBuild(body("pk_Chat-1")).shell.webchat).toBe("pk_Chat-1");
    expect(decodeBuild(body(undefined)).shell.webchat).toBeUndefined();
    for (const bad of ['pk" onload="x', "<vz-webchat>", "", 7]) expect(() => decodeBuild(body(bad))).toThrow(RequestInvalid);
  });

  test("every page carries one chat placement, and a page that places its own keeps only that one", async () => {
    for (const mode of ["preview", "publish"] as const) {
      const out = await build({ mode, project: htmlProject, shell: { ...shell, webchat: "pk_chat" }, runtime });
      const pages = out.assets.filter((a) => a.path.endsWith(".html") && !a.path.startsWith("__vz/"));
      expect(pages.length).toBeGreaterThan(0);
      for (const page of pages) {
        const html = new TextDecoder().decode(page.body);
        expect(html.match(/<vz-webchat\b/g)?.length).toBe(1);
        expect(html).toContain('<vz-webchat widget="pk_chat"></vz-webchat>');
      }
    }
    const placed = { ...htmlProject, files: { ...htmlProject.files } };
    const index = Object.keys(placed.files).find((path) => path.endsWith("index.html"))!;
    placed.files[index] = { ...placed.files[index], content: placed.files[index].content + '<vz-webchat widget="pk_own"></vz-webchat>' };
    const out = await build({ mode: "publish", project: placed, shell: { ...shell, webchat: "pk_chat" }, runtime });
    const html = new TextDecoder().decode(out.assets.find((a) => a.path === "index.html")!.body);
    expect(html.match(/<vz-webchat\b/g)?.length).toBe(1);
    expect(html).toContain('widget="pk_own"');
  });
});

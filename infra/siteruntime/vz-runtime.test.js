import { expect, test } from "bun:test";

import contract from "./testdata/runtime_contract.json";

import {
  attribution,
  consentSignals,
  countdownParts,
  evergreenDeadline,
  formFields,
  messagesFor,
  popupDue,
  readConsent,
  sendSubmission,
  TRAP_NAME,
  successAction,
  writeConsent,
} from "./vz-runtime.js";

const API = "https://api.vozkoia.com";
const NOW = Date.parse("2026-10-28T12:00:00Z");

function memoryStorage(seed = {}) {
  const data = new Map(Object.entries(seed));
  return {
    getItem: (key) => (data.has(key) ? data.get(key) : null),
    setItem: (key, value) => data.set(key, String(value)),
    data,
  };
}

function brokenStorage() {
  return {
    getItem() {
      throw new Error("blocked");
    },
    setItem() {
      throw new Error("blocked");
    },
  };
}

function fakeApi(responses) {
  const calls = [];
  const fetch = async (url, init) => {
    calls.push({ url, method: init.method, headers: init.headers, credentials: init.credentials, body: init.body ? JSON.parse(init.body) : undefined });
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return new Response(JSON.stringify(next.body), { status: next.status });
  };
  return { fetch, calls };
}

const challenge = { id: "c1", token: "t1", bits: 8, expiresAt: "2026-10-28T12:02:00Z" };

function submission(overrides = {}) {
  let clock = NOW;
  const waits = [];
  return {
    api: API,
    formKey: "fk_123",
    body: { idempotencyKey: "idem-1", website: "", route: "/", variant: "b", fields: { "lead.name": "Ana" }, utm: {}, referrer: "", fbclid: "", gclid: "", fbp: "", fbc: "" },
    solve: async (c) => `nonce-for-${c.token}`,
    now: () => clock,
    wait: async (ms) => {
      waits.push(ms);
      clock += ms;
    },
    waits,
    ...overrides,
  };
}

test("picks messages from the document language and falls back to Portuguese", () => {
  expect(messagesFor("en-US").success).toBe(messagesFor("en").success);
  expect(messagesFor("es").success).not.toBe(messagesFor("pt").success);
  expect(messagesFor("de-DE").success).not.toBe(messagesFor("pt").success);
  expect(messagesFor("fr").success).toBe(messagesFor("pt-BR").success);
  expect(messagesFor("").success).toBe(messagesFor("pt").success);
  for (const lang of ["pt", "en", "es", "de"]) expect(Object.keys(messagesFor(lang)).sort()).toEqual(Object.keys(messagesFor("pt")).sort());
});

test("collects the UTM set, click ids, Meta cookies and the variant", () => {
  const found = attribution({
    search: "?utm_source=instagram&utm_medium=paid&utm_campaign=sabado&utm_term=clareamento&utm_content=video1&fbclid=FB1&gclid=G1&other=x",
    cookie: "_fbp=fb.1.111.222; vz_variant=b; _fbc=fb.1.100.OLD",
    referrer: "https://www.instagram.com/",
    now: NOW,
  });
  expect(found).toEqual({
    utm: { source: "instagram", medium: "paid", campaign: "sabado", term: "clareamento", content: "video1" },
    referrer: "https://www.instagram.com/",
    fbclid: "FB1",
    gclid: "G1",
    fbp: "fb.1.111.222",
    fbc: "fb.1.100.OLD",
    variant: "b",
  });
});

test("builds the click cookie value from fbclid when Meta's cookie is missing", () => {
  const found = attribution({ search: "?fbclid=AbC", cookie: "", referrer: "", now: NOW });
  expect(found.fbc).toBe(`fb.1.${NOW}.AbC`);
  expect(found.utm).toEqual({ source: "", medium: "", campaign: "", term: "", content: "" });
  expect(found.variant).toBe("");
});

test("sends every named field except the honeypot and files", () => {
  const file = new File(["x"], "x.png");
  const entries = [
    ["lead.name", "Ana"],
    ["lead.phone", "+55 11 99999-0000"],
    ["field.interesses", "clareamento"],
    ["field.interesses", "implante"],
    ["vz_website", "bot"],
    ["field.foto", file],
  ];
  expect(formFields(entries)).toEqual({ "lead.name": "Ana", "lead.phone": "+55 11 99999-0000", "field.interesses": ["clareamento", "implante"] });
});

test("submits after solving a fresh challenge and waiting the minimum fill time", async () => {
  const api = fakeApi([{ status: 200, body: challenge }, { status: 201, body: { id: "s1", success: { kind: "message", message: "Até já!" } } }]);
  const run = submission({ fetch: api.fetch });
  const outcome = await sendSubmission(run);
  expect(outcome).toEqual({ ok: true, id: "s1", success: { kind: "message", message: "Até já!" } });
  expect(api.calls.map((c) => [c.method, c.url])).toEqual([
    ["POST", `${API}/public/sites/forms/fk_123/challenge`],
    ["POST", `${API}/public/sites/forms/fk_123/submissions`],
  ]);
  expect(api.calls[0].credentials).toBe("omit");
  expect(api.calls[1].headers["Content-Type"]).toBe("application/json");
  expect(api.calls[1].body).toEqual({ ...run.body, challengeToken: "t1", nonce: "nonce-for-t1" });
  expect(run.waits.reduce((a, b) => a + b, 0)).toBeGreaterThanOrEqual(3000);
});

test("reuses a challenge solved while the visitor was typing and does not wait again", async () => {
  const api = fakeApi([{ status: 201, body: { id: "s1" } }]);
  const run = submission({ fetch: api.fetch });
  const outcome = await sendSubmission({ ...run, proof: { challenge, nonce: "n9", fetchedAt: NOW - 20_000 } });
  expect(outcome).toEqual({ ok: true, id: "s1", success: { kind: "message" } });
  expect(api.calls).toHaveLength(1);
  expect(api.calls[0].body.nonce).toBe("n9");
  expect(run.waits).toEqual([]);
});

test("asks for a new challenge once when the server refuses the proof", async () => {
  const api = fakeApi([
    { status: 400, body: { error: true, code: "challenge_expired", message: "x" } },
    { status: 200, body: { ...challenge, token: "t2" } },
    { status: 201, body: { id: "s1" } },
  ]);
  const outcome = await sendSubmission({ ...submission({ fetch: api.fetch }), proof: { challenge, nonce: "n1", fetchedAt: NOW - 10_000 } });
  expect(outcome.ok).toBe(true);
  expect(api.calls[2].body.challengeToken).toBe("t2");
});

test("reports refusals without ever claiming success", async () => {
  const invalid = fakeApi([{ status: 200, body: challenge }, { status: 422, body: { error: true, code: "form_field_required", message: "x", details: { "lead.phone": ["required"] } } }]);
  expect(await sendSubmission(submission({ fetch: invalid.fetch }))).toEqual({ ok: false, reason: "invalid", fields: ["lead.phone"] });

  const limited = fakeApi([{ status: 200, body: challenge }, { status: 429, body: { error: true, code: "rate_limited", message: "x" } }]);
  expect(await sendSubmission(submission({ fetch: limited.fetch }))).toEqual({ ok: false, reason: "rate_limited", fields: [] });

  const offline = fakeApi([{ status: 200, body: challenge }, new TypeError("Failed to fetch")]);
  expect(await sendSubmission(submission({ fetch: offline.fetch }))).toEqual({ ok: false, reason: "failed", fields: [] });

  const garbled = fakeApi([{ status: 200, body: challenge }, { status: 200, body: "ok" }]);
  expect(await sendSubmission(submission({ fetch: garbled.fetch }))).toEqual({ ok: false, reason: "failed", fields: [] });

  const badChallenge = fakeApi([{ status: 200, body: { token: "" } }]);
  expect(await sendSubmission(submission({ fetch: badChallenge.fetch, solve: async () => { throw new Error("refused"); } }))).toEqual({ ok: false, reason: "failed", fields: [] });
});

test("refuses to send without an API origin or a form key", async () => {
  const api = fakeApi([]);
  expect(await sendSubmission(submission({ fetch: api.fetch, api: "" }))).toEqual({ ok: false, reason: "unavailable", fields: [] });
  expect(await sendSubmission(submission({ fetch: api.fetch, api: "javascript:alert(1)" }))).toEqual({ ok: false, reason: "unavailable", fields: [] });
  expect(await sendSubmission(submission({ fetch: api.fetch, formKey: "" }))).toEqual({ ok: false, reason: "unavailable", fields: [] });
  expect(await sendSubmission(submission({ fetch: api.fetch, formKey: "../x" }))).toEqual({ ok: false, reason: "unavailable", fields: [] });
  expect(api.calls).toEqual([]);
});

test("accepts only safe success actions", () => {
  expect(successAction({ kind: "redirect", route: "/obrigado" })).toEqual({ kind: "redirect", route: "/obrigado" });
  expect(successAction({ kind: "redirect", route: "//evil.example" })).toEqual({ kind: "message" });
  expect(successAction({ kind: "redirect", route: "https://evil.example" })).toEqual({ kind: "message" });
  expect(successAction({ kind: "redirect", route: "/\\evil.example" })).toEqual({ kind: "message" });
  expect(successAction({ kind: "redirect", route: "/obrigado\\x" })).toEqual({ kind: "message" });
  expect(successAction({ kind: "redirect", route: "/obrigado" })).toEqual({ kind: "redirect", route: "/obrigado" });
  expect(successAction({ kind: "whatsapp", url: "https://wa.me/5511999990000?text=oi" })).toEqual({ kind: "whatsapp", url: "https://wa.me/5511999990000?text=oi" });
  expect(successAction({ kind: "whatsapp", url: "javascript:alert(1)" })).toEqual({ kind: "message" });
  expect(successAction({ kind: "message", message: "Obrigado!" })).toEqual({ kind: "message", message: "Obrigado!" });
  expect(successAction(undefined)).toEqual({ kind: "message" });
  expect(successAction({ kind: "explode" })).toEqual({ kind: "message" });
});

test("stores the consent choice on the device and survives blocked storage", () => {
  const storage = memoryStorage();
  expect(readConsent(storage)).toBeNull();
  writeConsent(storage, { analytics: true, marketing: false }, NOW);
  expect(readConsent(storage)).toEqual({ analytics: true, marketing: false });
  expect(readConsent(memoryStorage({ "vz-consent": "{not json" }))).toBeNull();
  expect(readConsent(memoryStorage({ "vz-consent": JSON.stringify({ v: 1, analytics: "yes", marketing: false }) }))).toBeNull();
  expect(readConsent(brokenStorage())).toBeNull();
  expect(() => writeConsent(brokenStorage(), { analytics: true, marketing: true }, NOW)).not.toThrow();
});

test("maps consent categories to Google Consent Mode v2 signals, denied by default", () => {
  expect(consentSignals(null)).toEqual({ analytics_storage: "denied", ad_storage: "denied", ad_user_data: "denied", ad_personalization: "denied" });
  expect(consentSignals({ analytics: true, marketing: false })).toEqual({ analytics_storage: "granted", ad_storage: "denied", ad_user_data: "denied", ad_personalization: "denied" });
  expect(consentSignals({ analytics: false, marketing: true })).toEqual({ analytics_storage: "denied", ad_storage: "granted", ad_user_data: "granted", ad_personalization: "granted" });
});

test("shows a popup again only after its frequency cap", () => {
  const day = 86_400_000;
  expect(popupDue(null, NOW, 7)).toBe(true);
  expect(popupDue(String(NOW - 2 * day), NOW, 7)).toBe(false);
  expect(popupDue(String(NOW - 8 * day), NOW, 7)).toBe(true);
  expect(popupDue(String(NOW - 1000), NOW, 0)).toBe(true);
  expect(popupDue("garbage", NOW, 7)).toBe(true);
});

test("splits the time left into days, hours, minutes and seconds", () => {
  expect(countdownParts(NOW + ((2 * 24 + 3) * 3600 + 4 * 60 + 5) * 1000 + 400, NOW)).toEqual({ days: 2, hours: 3, minutes: 4, seconds: 5, done: false });
  expect(countdownParts(NOW - 1, NOW)).toEqual({ days: 0, hours: 0, minutes: 0, seconds: 0, done: true });
});

test("gives each visitor their own evergreen deadline and keeps it", () => {
  const storage = memoryStorage();
  const first = evergreenDeadline(storage, "vz-countdown:oferta", 30, NOW);
  expect(first).toBe(NOW + 30 * 60_000);
  expect(evergreenDeadline(storage, "vz-countdown:oferta", 30, NOW + 5000)).toBe(first);
  expect(evergreenDeadline(brokenStorage(), "vz-countdown:oferta", 30, NOW)).toBe(NOW + 30 * 60_000);
});

test("the trap field matches the name the form inspector ignores", () => {
  expect(TRAP_NAME).toBe(contract.trapField);
});

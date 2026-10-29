import { afterEach, expect, test } from "bun:test";
import { Window } from "happy-dom";

import { startRuntime } from "./vz-runtime.js";

const API = "https://api.vozkoia.com";
const NOW = Date.parse("2026-10-28T12:00:00Z");
const opened = [];

afterEach(async () => {
  while (opened.length) await opened.pop().happyDOM.abort();
});

function tick(times = 8) {
  let chain = Promise.resolve();
  for (let i = 0; i < times; i++) chain = chain.then(() => new Promise((done) => setTimeout(done, 0)));
  return chain;
}

async function until(check, rounds = 200) {
  for (let i = 0; i < rounds && !check(); i++) await tick(1);
}

const slots = (preview = false) => ({
  status: 200,
  body: {
    timeZone: "America/Sao_Paulo",
    durationMinutes: 30,
    fields: ["email"],
    consentText: "Autorizo o contato pelo WhatsApp.",
    preview,
    slots: [
      { start: "2026-10-29T12:00:00Z", end: "2026-10-29T12:30:00Z" },
      { start: "2026-10-29T12:30:00Z", end: "2026-10-29T13:00:00Z" },
      { start: "2026-10-30T12:00:00Z", end: "2026-10-30T12:30:00Z" },
    ],
  },
});

const challenge = { status: 200, body: { id: "c1", token: "c1", bits: 8, expiresAt: "2026-10-28T12:02:00Z" } };

function setup({ html = '<vz-booking booking="agenda"><p>Agende pelo WhatsApp</p></vz-booking>', forms = { agenda: "fk_agenda" }, responses = [] } = {}) {
  const window = new Window({ url: "https://clinica.vozko.app/", settings: { navigator: { userAgent: "test" } } });
  opened.push(window);
  const doc = window.document;
  doc.documentElement.lang = "pt-BR";
  doc.documentElement.setAttribute("data-vz-api", API);
  doc.documentElement.setAttribute("data-vz-forms", JSON.stringify(forms));
  Object.defineProperty(window.navigator, "sendBeacon", { value: () => true });
  doc.body.innerHTML = html;
  const calls = [];
  const fetch = async (url, init = {}) => {
    calls.push({ url, method: init.method || "GET", body: init.body ? JSON.parse(init.body) : undefined });
    const next = responses.shift();
    if (!next) throw new Error("no response queued for " + url);
    return new Response(JSON.stringify(next.body), { status: next.status });
  };
  let clock = NOW;
  const navigations = [];
  const leads = [];
  window.addEventListener("vz:lead", (event) => leads.push(event.detail));
  startRuntime(window, {
    fetch,
    solve: async (c) => "nonce-" + c.token,
    now: () => clock,
    wait: async (ms) => {
      clock += ms;
    },
    go: (target) => navigations.push(target),
    timeZone: "America/Sao_Paulo",
  });
  const host = doc.querySelector("vz-booking");
  return { window, doc, host, calls, navigations, leads, responses };
}

const buttons = (host, group) => [...host.querySelectorAll(`[data-vz-${group}] button`)];

test("shows the visitor's days and times, then the contact fields for the chosen time", async () => {
  const page = setup({ responses: [slots()] });
  expect(page.host.textContent).toContain("Carregando");
  await until(() => buttons(page.host, "days").length > 0);
  expect(page.calls[0]).toEqual({ url: API + "/public/sites/forms/fk_agenda/slots", method: "GET", body: undefined });
  const days = buttons(page.host, "days");
  expect(days.length).toBe(2);
  expect(days[0].getAttribute("aria-pressed")).toBe("true");
  expect(buttons(page.host, "times").map((b) => b.textContent)).toEqual(["09:00", "09:30"]);
  expect(page.host.querySelector("form").hidden).toBe(true);
  expect(page.host.textContent).toContain("America/Sao_Paulo");
  days[1].click();
  expect(buttons(page.host, "times").map((b) => b.textContent)).toEqual(["09:00"]);
  days[0].click();
  buttons(page.host, "times")[1].click();
  const form = page.host.querySelector("form");
  expect(form.hidden).toBe(false);
  expect(buttons(page.host, "times")[1].getAttribute("aria-pressed")).toBe("true");
  expect([...form.querySelectorAll("input,textarea")].map((i) => i.name)).toEqual(["lead.name", "lead.phone", "lead.email", "consent", "vz_website"]);
  expect(form.textContent).toContain("Autorizo o contato pelo WhatsApp.");
  expect(form.querySelector('[name="lead.name"]').required).toBe(true);
});

test("books the chosen time and shows the confirmation with the meeting link", async () => {
  const booked = { status: 200, body: { id: "sub-9", success: { kind: "message" }, booking: { start: "2026-10-29T12:30:00Z", end: "2026-10-29T13:00:00Z", timeZone: "America/Sao_Paulo", meetingLink: "https://meet.google.com/abc" } } };
  const page = setup({ responses: [slots(), challenge, booked] });
  await until(() => buttons(page.host, "times").length > 0);
  buttons(page.host, "times")[1].click();
  const form = page.host.querySelector("form");
  form.querySelector('[name="lead.name"]').value = "Ana";
  form.querySelector('[name="lead.phone"]').value = "11999990000";
  form.querySelector('[name="consent"]').checked = true;
  form.dispatchEvent(new page.window.Event("submit", { bubbles: true, cancelable: true }));
  await until(() => page.leads.length > 0);
  const sent = page.calls.find((c) => c.url.endsWith("/bookings"));
  expect(sent.body.fields["field.horario"]).toBe("2026-10-29T12:30:00Z");
  expect(sent.body.fields["lead.name"]).toBe("Ana");
  expect(sent.body.fields.consent).toBe("on");
  expect(sent.body.challengeToken).toBe("c1");
  expect(page.leads).toEqual([{ eventId: "sub-9", form: "agenda" }]);
  const link = page.host.querySelector('a[href="https://meet.google.com/abc"]');
  expect(link).not.toBeNull();
  expect(page.host.querySelector("[data-vz-status]").textContent).toContain("09:30");
});

test("a taken time reloads the free slots and keeps what the visitor typed", async () => {
  const page = setup({ responses: [slots(), challenge, { status: 409, body: { code: "booking_slot_taken" } }, slots()] });
  await until(() => buttons(page.host, "times").length > 0);
  buttons(page.host, "times")[0].click();
  const form = page.host.querySelector("form");
  form.querySelector('[name="lead.name"]').value = "Ana";
  form.querySelector('[name="lead.phone"]').value = "11999990000";
  form.querySelector('[name="consent"]').checked = true;
  form.dispatchEvent(new page.window.Event("submit", { bubbles: true, cancelable: true }));
  await until(() => page.calls.length >= 4 && buttons(page.host, "times").length > 0);
  expect(page.calls.filter((c) => c.url.endsWith("/slots")).length).toBe(2);
  expect(page.host.querySelector("[data-vz-status]").textContent).toContain("reservado");
  expect(page.host.querySelector('[name="lead.name"]').value).toBe("Ana");
  expect(page.leads).toEqual([]);
});

test("an unreadable calendar or a missing key shows the unavailable state", async () => {
  const down = setup({ responses: [{ status: 503, body: { code: "booking_unavailable" } }] });
  await until(() => down.host.textContent.includes("indisponível"));
  expect(down.host.querySelector("form")).toBeNull();
  const keyless = setup({ forms: {} });
  await tick();
  expect(keyless.calls).toEqual([]);
  expect(keyless.host.textContent).toContain("indisponível");
});

test("a preview says the times are examples", async () => {
  const page = setup({ forms: { agenda: "vz-preview" }, responses: [slots(true)] });
  await until(() => buttons(page.host, "times").length > 0);
  expect(page.calls[0].url).toBe(API + "/public/sites/forms/vz-preview/slots");
  expect(page.host.textContent).toContain("Prévia");
});

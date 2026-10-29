import { afterEach, expect, test } from "bun:test";
import { Window } from "happy-dom";

import { startRuntime } from "./vz-runtime.js";

const API = "https://api.vozkoia.com";
const NOW = Date.parse("2026-10-28T12:00:00Z");
const PAGE_URL = "https://clinica.vozko.site/oferta?utm_source=instagram&utm_campaign=sabado&fbclid=FBCLID1&gclid=GCLID1";
const FORM = '<form data-vz-form="contato"><input name="lead.name" value="Ana"><input name="lead.phone" value="11999990000"><button type="submit">Enviar</button></form>';
const opened = [];

afterEach(async () => {
  while (opened.length) await opened.pop().happyDOM.abort();
});

function tick(times = 6) {
  let chain = Promise.resolve();
  for (let i = 0; i < times; i++) chain = chain.then(() => new Promise((done) => setTimeout(done, 0)));
  return chain;
}

function reply(status, body) {
  return { status, body };
}

const challenge = (token) => reply(200, { id: token, token, bits: 8, expiresAt: "2026-10-28T12:02:00Z" });

function setup({ html = FORM, url = PAGE_URL, lang = "pt-BR", forms = { contato: "fk_contato" }, responses = [], stored = {}, cookie = "", referrer = "", preview = false } = {}) {
  const window = new Window({ url, settings: { navigator: { userAgent: "test" } } });
  opened.push(window);
  const doc = window.document;
  const root = doc.documentElement;
  root.lang = lang;
  root.setAttribute("data-vz-api", API);
  root.setAttribute("data-vz-forms", JSON.stringify(forms));
  if (preview) root.setAttribute("data-vz-preview", "");
  for (const [key, value] of Object.entries(stored)) window.localStorage.setItem(key, value);
  if (cookie) doc.cookie = cookie;
  if (referrer) Object.defineProperty(doc, "referrer", { value: referrer });
  doc.body.innerHTML = html;
  const beacons = [];
  Object.defineProperty(window.navigator, "sendBeacon", { value: (path, blob) => beacons.push({ path, blob }) > 0 });
  const calls = [];
  const fetch = async (requestUrl, init) => {
    calls.push({ url: requestUrl, body: JSON.parse(init.body) });
    const next = responses.shift();
    if (!next) throw new Error("no response queued for " + requestUrl);
    return new Response(JSON.stringify(next.body), { status: next.status });
  };
  let clock = NOW;
  const navigations = [];
  startRuntime(window, {
    fetch,
    solve: async (c) => "nonce-" + c.token,
    now: () => clock,
    wait: async (ms) => {
      clock += ms;
    },
    go: (target) => navigations.push(target),
  });
  return {
    window,
    doc,
    calls,
    navigations,
    payloads: () => Promise.all(beacons.map(async (b) => ({ path: b.path, ...JSON.parse(await b.blob.text()) }))),
  };
}

async function submit(page, form = page.doc.querySelector("form")) {
  form.dispatchEvent(new page.window.Event("submit", { bubbles: true, cancelable: true }));
  for (let i = 0; i < 50 && !form.hasAttribute("aria-busy"); i++) await tick(1);
  for (let i = 0; i < 200 && form.hasAttribute("aria-busy"); i++) await tick(1);
}

function focusIn(page, element) {
  element.dispatchEvent(new page.window.FocusEvent("focusin", { bubbles: true }));
}

const status = (page) => page.doc.querySelector("[data-vz-status]");
const traps = (form) => form.querySelectorAll('input[name="vz_website"]').length;

test("a preview counts nothing, while a published page sends its visit", async () => {
  const preview = setup({ preview: true });
  await tick(5);
  expect(await preview.payloads()).toEqual([]);
  const published = setup();
  await tick(5);
  expect((await published.payloads()).map((p) => p.event)).toContain("view");
});

test("attaches once to custom element forms and plain markup forms, even when React replaces or moves them", async () => {
  const page = setup({ html: '<vz-form id="contato"><form><input name="lead.name"></form></vz-form>' + FORM });
  await tick();
  const [inside, plain] = page.doc.querySelectorAll("form");
  expect(traps(inside)).toBe(1);
  expect(traps(plain)).toBe(1);

  const parent = plain.parentNode;
  plain.remove();
  parent.appendChild(plain);
  await tick();
  expect(traps(plain)).toBe(1);

  const rerendered = page.doc.createElement("form");
  rerendered.setAttribute("data-vz-form", "contato");
  rerendered.innerHTML = '<input name="lead.name">';
  plain.replaceWith(rerendered);
  await tick();
  expect(traps(rerendered)).toBe(1);
  expect(page.window.customElements.get("vz-form")).toBeDefined();
});

test("solves the challenge while the visitor types and confirms only what the API accepted", async () => {
  const page = setup({ cookie: "vz_variant=b", responses: [challenge("t1"), reply(201, { id: "s1" })] });
  await tick();
  const name = page.doc.querySelector('[name="lead.name"]');
  focusIn(page, name);
  name.value = "Bia";
  await tick();
  expect(page.calls.map((c) => c.url)).toEqual([`${API}/public/sites/forms/fk_contato/challenge`]);

  await submit(page);
  const sent = page.calls[1];
  expect(sent.url).toBe(`${API}/public/sites/forms/fk_contato/submissions`);
  expect(sent.body).toMatchObject({
    challengeToken: "t1",
    nonce: "nonce-t1",
    website: "",
    route: "/oferta",
    variant: "b",
    fields: { "lead.name": "Bia", "lead.phone": "11999990000" },
    utm: { source: "instagram", medium: "", campaign: "sabado", term: "", content: "" },
    fbclid: "FBCLID1",
    gclid: "GCLID1",
  });
  expect(sent.body.idempotencyKey).toMatch(/^[0-9a-f-]{36}$/);
  expect(status(page).textContent).toBe("Recebemos seus dados. Obrigado!");
  expect(status(page).getAttribute("aria-live")).toBe("polite");
  expect(name.value).toBe("Ana");
  expect((await page.payloads()).map((p) => p.event)).toEqual(["view", "form_start", "form_submit"]);
});

test("asks for a new challenge once when the proof is refused, then gives up", async () => {
  const retried = setup({ responses: [challenge("t1"), reply(400, { error: true, code: "challenge_expired" }), challenge("t2"), reply(201, { id: "s1", success: { kind: "message", message: "Até já!" } })] });
  await submit(retried);
  expect(retried.calls.map((c) => c.body.challengeToken)).toEqual([undefined, "t1", undefined, "t2"]);
  expect(status(retried).textContent).toBe("Até já!");

  const refused = setup({ responses: [challenge("t1"), reply(400, { error: true, code: "challenge_reused" }), challenge("t2"), reply(400, { error: true, code: "challenge_reused" })] });
  await submit(refused);
  expect(refused.calls).toHaveLength(4);
  expect(status(refused).getAttribute("role")).toBe("alert");
  expect(status(refused).textContent).toBe("Não foi possível enviar. Tente de novo.");
});

test("marks the fields the API refused and moves focus to the first one", async () => {
  const page = setup({ responses: [challenge("t1"), reply(422, { error: true, code: "form_field_invalid", details: { "lead.phone": ["invalid"] } })] });
  await submit(page);
  const phone = page.doc.querySelector('[name="lead.phone"]');
  expect(phone.getAttribute("aria-invalid")).toBe("true");
  expect(page.doc.activeElement).toBe(phone);
  expect(status(page).getAttribute("role")).toBe("alert");
  expect(status(page).textContent).toBe("Confira os campos destacados.");
  expect(page.doc.querySelector('[name="lead.name"]').value).toBe("Ana");
  expect((await page.payloads()).map((p) => p.event)).not.toContain("form_submit");
});

test("tells the visitor to wait when the API rate limits, in the page language", async () => {
  const page = setup({ lang: "en", responses: [challenge("t1"), reply(429, { error: true, code: "rate_limited" })] });
  await submit(page);
  expect(status(page).textContent).toBe("Too many attempts. Please wait a moment.");
});

test("follows the success action the API returns", async () => {
  const redirect = setup({ responses: [challenge("t1"), reply(201, { id: "s1", success: { kind: "redirect", route: "/obrigado" } })] });
  await submit(redirect);
  expect(redirect.navigations).toEqual(["/obrigado"]);

  const whatsapp = setup({ responses: [challenge("t1"), reply(201, { id: "s1", success: { kind: "whatsapp", url: "https://wa.me/5511999990000?text=oi" } })] });
  await submit(whatsapp);
  expect(whatsapp.navigations).toEqual(["https://wa.me/5511999990000?text=oi"]);
  expect((await whatsapp.payloads()).map((p) => p.event)).toContain("whatsapp");

  const unsafe = setup({ responses: [challenge("t1"), reply(201, { id: "s1", success: { kind: "redirect", route: "//evil.example" } })] });
  await submit(unsafe);
  expect(unsafe.navigations).toEqual([]);
  expect(status(unsafe).textContent).toBe("Recebemos seus dados. Obrigado!");
});

test("refuses to send a form whose key the build did not write", async () => {
  const page = setup({ forms: {} });
  await submit(page);
  expect(page.calls).toEqual([]);
  expect(status(page).textContent).toBe("Formulário indisponível no momento.");
});

function consentPage(options = {}) {
  return setup({
    html: '<a href="#" data-vz-consent-open>Cookies</a><vz-consent privacy="/privacidade"></vz-consent><script type="text/plain" data-vz-consent="analytics" data-kind="ga">window.ranAnalytics = 1</script><script type="text/plain" data-vz-consent="marketing" data-kind="pixel">window.ranMarketing = 1</script>',
    ...options,
  });
}

const banner = (page) => page.doc.querySelector("vz-consent");
const buttonNamed = (page, label) => Array.from(banner(page).querySelectorAll("button")).find((b) => b.textContent === label);
const script = (page, kind) => page.doc.querySelector(`script[data-kind="${kind}"]`);
const consentCalls = (page) => (page.window.dataLayer || []).map((entry) => Array.from(entry));

test("starts with every consent signal denied and shows reject as plainly as accept", async () => {
  const page = consentPage();
  await tick();
  expect(banner(page).hidden).toBe(false);
  expect(Object.prototype.toString.call(page.window.dataLayer[0])).toBe("[object Arguments]");
  expect(consentCalls(page)[0]).toEqual(["consent", "default", { analytics_storage: "denied", ad_storage: "denied", ad_user_data: "denied", ad_personalization: "denied", wait_for_update: 500 }]);
  const reject = buttonNamed(page, "Recusar opcionais");
  const accept = buttonNamed(page, "Aceitar todos");
  expect(reject.tagName).toBe(accept.tagName);
  expect(reject.className).toBe(accept.className);
  expect(banner(page).querySelector('a[href="/privacidade"]')).not.toBeNull();
  expect(script(page, "ga").getAttribute("type")).toBe("text/plain");
});

test("accepting activates every gated script and records the choice on the device", async () => {
  const page = consentPage();
  await tick();
  buttonNamed(page, "Aceitar todos").click();
  expect(banner(page).hidden).toBe(true);
  expect(script(page, "ga").hasAttribute("type")).toBe(false);
  expect(script(page, "pixel").hasAttribute("type")).toBe(false);
  expect(script(page, "ga").hasAttribute("data-vz-consent")).toBe(false);
  expect(consentCalls(page).at(-1)).toEqual(["consent", "update", { analytics_storage: "granted", ad_storage: "granted", ad_user_data: "granted", ad_personalization: "granted" }]);
  expect(JSON.parse(page.window.localStorage.getItem("vz-consent"))).toMatchObject({ v: 1, analytics: true, marketing: true });
});

test("rejecting keeps every gated script inert", async () => {
  const page = consentPage();
  await tick();
  buttonNamed(page, "Recusar opcionais").click();
  expect(script(page, "ga").getAttribute("type")).toBe("text/plain");
  expect(script(page, "pixel").getAttribute("type")).toBe("text/plain");
  expect(consentCalls(page).at(-1)[2]).toEqual({ analytics_storage: "denied", ad_storage: "denied", ad_user_data: "denied", ad_personalization: "denied" });
});

test("applies a stored choice by category and lets the visitor reopen and change it", async () => {
  const page = consentPage({ stored: { "vz-consent": JSON.stringify({ v: 1, analytics: true, marketing: false }) } });
  await tick();
  expect(banner(page).hidden).toBe(true);
  expect(script(page, "ga").hasAttribute("type")).toBe(false);
  expect(script(page, "pixel").getAttribute("type")).toBe("text/plain");

  page.doc.querySelector("[data-vz-consent-open]").click();
  expect(banner(page).hidden).toBe(false);
  buttonNamed(page, "Personalizar").click();
  const marketing = banner(page).querySelector('input[name="marketing"]');
  expect(banner(page).querySelector('input[name="analytics"]').checked).toBe(true);
  expect(banner(page).querySelector('input[name="necessary"]').disabled).toBe(true);
  marketing.checked = true;
  buttonNamed(page, "Salvar escolhas").click();
  expect(banner(page).hidden).toBe(true);
  expect(script(page, "pixel").hasAttribute("type")).toBe(false);
  expect(consentCalls(page).at(-1)[2].ad_storage).toBe("granted");
});

test("counts down to a deadline, fills unit slots and runs the expired action", async () => {
  const deadline = new Date(NOW + ((2 * 24 + 3) * 3600 + 4 * 60 + 5) * 1000).toISOString();
  const page = setup({
    html: `<vz-countdown deadline="${deadline}"></vz-countdown><div data-vz-countdown data-deadline="${deadline}"><span data-unit="days"></span>:<span data-unit="seconds"></span></div><vz-countdown id="fim" deadline="2026-10-01T00:00:00Z" expired="hide"></vz-countdown><vz-countdown deadline="2026-10-01T00:00:00Z" expired="redirect" expired-route="/encerrada"></vz-countdown>`,
  });
  await tick();
  const [text, slots, hidden] = page.doc.querySelectorAll("vz-countdown, [data-vz-countdown]");
  expect(text.textContent).toBe("2d 03:04:05");
  expect(text.getAttribute("role")).toBe("timer");
  expect(Array.from(slots.querySelectorAll("[data-unit]")).map((s) => s.textContent)).toEqual(["02", "05"]);
  expect(hidden.hidden).toBe(true);
  expect(page.navigations).toEqual(["/encerrada"]);
});

test("gives each visitor an evergreen deadline that survives a reload", async () => {
  const first = setup({ html: '<vz-countdown id="oferta" evergreen="30"></vz-countdown>' });
  await tick();
  expect(first.doc.querySelector("vz-countdown").textContent).toBe("00:30:00");
  const kept = first.window.localStorage.getItem("vz-countdown:oferta");
  expect(Number(kept)).toBe(NOW + 30 * 60000);
});

test("opens a timed popup as a native modal dialog, closes it and returns focus", async () => {
  const page = setup({ html: '<button id="antes">antes</button><dialog id="promo" data-vz-popup="timer" data-delay="0"><p>Oferta</p><button data-vz-close>Fechar</button></dialog>' });
  const before = page.doc.getElementById("antes");
  before.focus();
  await tick(10);
  const dialog = page.doc.getElementById("promo");
  expect(dialog.open).toBe(true);
  expect(page.doc.activeElement).toBe(dialog.querySelector("button"));
  dialog.querySelector("[data-vz-close]").click();
  expect(dialog.open).toBe(false);
  expect(page.doc.activeElement).toBe(before);
  expect(Number(page.window.localStorage.getItem("vz-popup:promo"))).toBe(NOW);
});

test("respects the popup frequency cap per visitor", async () => {
  const page = setup({ html: '<dialog id="promo" data-vz-popup="timer" data-delay="0" data-frequency="7"><button>ok</button></dialog>', stored: { "vz-popup:promo": String(NOW - 86400000) } });
  await tick(10);
  expect(page.doc.getElementById("promo").open).toBe(false);
});

test("opens on exit intent and closes on the dialog's cancel event", async () => {
  const page = setup({ html: '<vz-popup trigger="exit" frequency="0"><dialog id="saida"><button>Fique</button></dialog></vz-popup>' });
  await tick();
  const dialog = page.doc.getElementById("saida");
  page.doc.dispatchEvent(new page.window.MouseEvent("mouseout", { bubbles: true, clientY: 20 }));
  expect(dialog.open).toBe(false);
  page.doc.dispatchEvent(new page.window.MouseEvent("mouseout", { bubbles: true, clientY: -1 }));
  expect(dialog.open).toBe(true);
  dialog.dispatchEvent(new page.window.Event("cancel", { cancelable: true }));
  expect(dialog.open).toBe(false);
});

test("opens on scroll depth", async () => {
  const page = setup({ html: '<dialog id="rolagem" data-vz-popup="scroll" data-depth="50"><button>ok</button></dialog>' });
  await tick();
  page.window.dispatchEvent(new page.window.Event("scroll"));
  expect(page.doc.getElementById("rolagem").open).toBe(true);
});

test("counts views, CTA clicks and WhatsApp clicks without any identifier", async () => {
  const page = setup({
    html: '<a href="#plano" data-vz-cta="hero">Quero</a><a data-vz-whatsapp href="https://vzk.to/abc">WhatsApp</a>',
    cookie: "vz_variant=b; _fbp=fb.1.111.SECRETFBP; _fbc=fb.1.1.SECRETFBC",
    referrer: "https://www.instagram.com/stories/clinica/123?igsh=SECRETIG",
  });
  await tick();
  page.doc.querySelector("[data-vz-cta]").click();
  page.doc.querySelector("[data-vz-whatsapp]").click();
  const payloads = await page.payloads();
  expect(payloads.map((p) => p.path)).toEqual(["/__vz/e", "/__vz/e", "/__vz/e"]);
  expect(payloads.map((p) => Object.keys(p).filter((key) => key !== "path").sort())).toEqual([
    ["event", "referrer", "route", "utm", "variant"],
    ["event", "route", "target", "variant"],
    ["event", "route", "target", "variant"],
  ]);
  expect(payloads[0]).toMatchObject({ event: "view", route: "/oferta", variant: "b", referrer: "https://www.instagram.com" });
  expect(payloads[1]).toMatchObject({ event: "cta", target: "hero" });
  expect(payloads[2]).toMatchObject({ event: "whatsapp", target: "https://vzk.to/abc" });
  const serialized = JSON.stringify(payloads);
  for (const secret of ["FBCLID1", "GCLID1", "SECRETFBP", "SECRETFBC", "SECRETIG", "fbclid", "gclid", "fbp", "fbc"]) expect(serialized).not.toContain(secret);
});

function listen(page, type) {
  const seen = [];
  page.window.addEventListener(type, (event) => seen.push(event.detail));
  return seen;
}

test("a submitted form signals the lead with the server submission id for consent gated pixels", async () => {
  const page = setup({ responses: [challenge("t1"), reply(201, { id: "sub-42", success: { kind: "message" } })] });
  const leads = listen(page, "vz:lead");
  await submit(page);
  expect(leads).toEqual([{ eventId: "sub-42", form: "contato" }]);

  const refused = setup({ responses: [challenge("t1"), reply(422, { code: "submission_fields_invalid", details: { "lead.phone": "invalid" } })] });
  const none = listen(refused, "vz:lead");
  await submit(refused);
  expect(none).toEqual([]);
});

test("a WhatsApp click and a WhatsApp success signal a contact", async () => {
  const page = setup({ html: '<a data-vz-whatsapp href="https://vzk.to/abc">WhatsApp</a>' });
  const contacts = listen(page, "vz:contact");
  await tick();
  page.doc.querySelector("[data-vz-whatsapp]").click();
  expect(contacts).toEqual([{ target: "https://vzk.to/abc" }]);

  const success = setup({ responses: [challenge("t1"), reply(201, { id: "s1", success: { kind: "whatsapp", url: "https://wa.me/5511999990000?text=oi" } })] });
  const after = listen(success, "vz:contact");
  await submit(success);
  expect(after).toEqual([{ target: "https://wa.me/5511999990000?text=oi" }]);
});

test("a published chat loads the workspace webchat loader once with the widget public key", async () => {
  const page = setup({ html: '<vz-webchat widget="wc" data-vz-webchat-key="pk_Abc-1"></vz-webchat><vz-webchat widget="wc2" data-vz-webchat-key="pk_other"></vz-webchat>' });
  await tick();
  const loaders = page.doc.querySelectorAll("script[data-vz-webchat]");
  expect(loaders.length).toBe(1);
  expect(loaders[0].getAttribute("src")).toBe(API + "/public/webchat/loader.js");
  expect(loaders[0].getAttribute("data-key")).toBe("pk_Abc-1");
});

test("a chat without a resolved key, with a forged key or on a bad api origin loads nothing", async () => {
  for (const html of ['<vz-webchat widget="wc"></vz-webchat>', '<vz-webchat data-vz-webchat-key="bad key&quot;"></vz-webchat>']) {
    const page = setup({ html });
    await tick();
    expect(page.doc.querySelectorAll("script").length).toBe(0);
  }
  const page = setup({ html: "<main></main>" });
  page.doc.documentElement.setAttribute("data-vz-api", "http://evil.example");
  page.doc.querySelector("main").innerHTML = '<vz-webchat data-vz-webchat-key="pk_2"></vz-webchat>';
  await tick();
  expect(page.doc.querySelectorAll("script").length).toBe(0);
});

test("a preview chat loads the same live loader so the server can answer it as a demo", async () => {
  const page = setup({ html: '<vz-webchat widget="wc" data-vz-webchat-key="pk_1"></vz-webchat>', lang: "en" });
  await tick();
  const chat = page.doc.querySelector("vz-webchat");
  const loaders = page.doc.querySelectorAll("script[data-vz-webchat]");
  expect(loaders.length).toBe(1);
  expect(loaders[0].getAttribute("src")).toBe(API + "/public/webchat/loader.js");
  expect(loaders[0].getAttribute("data-key")).toBe("pk_1");
  expect(chat.getAttribute("role")).toBe(null);
  expect(chat.innerHTML).toBe("");
});

test("a hand written preview marker loads nothing", async () => {
  const page = setup({ html: '<vz-webchat widget="wc" data-vz-webchat-preview="pk_1"></vz-webchat>' });
  await tick();
  expect(page.doc.querySelectorAll("script").length).toBe(0);
  expect(page.doc.querySelector("vz-webchat").innerHTML).toBe("");
});

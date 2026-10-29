import { solveChallenge } from "./pow.js";

const MIN_FILL_MS = 3250;
const PROOF_REUSE_MS = 90000;
const DAY_MS = 86400000;
export const TRAP_NAME = "vz_website";
const VZ = Symbol("vz");
const CONSENT_KEY = "vz-consent";
const BEACON_PATH = "/__vz/e";
const PREVIEW_ATTR = "data-vz-preview";
const FORM_KEY = /^[A-Za-z0-9_-]{1,128}$/;
const LOCAL_ROUTE = /^\/(?![/\\])[^\\]*$/;
const UTM = ["source", "medium", "campaign", "term", "content"];
const FOCUSABLE = "a[href],button,input,select,textarea,[tabindex]";
const WEBCHAT_LOADER = "/public/webchat/loader.js";
const WEBCHAT_KEY = /^[A-Za-z0-9_-]{1,128}$/;
const SLOT_FIELD = "field.horario";
const SLOT_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;
const BOOKING_FIELDS = ["email", "message"];
const BOOKING_CSS = "vz-booking{display:block}vz-booking [data-vz-booking-root]{display:grid;gap:1rem}vz-booking [data-vz-days]{display:flex;gap:.5rem;overflow-x:auto;padding-bottom:.25rem}vz-booking [data-vz-times]{display:grid;grid-template-columns:repeat(auto-fill,minmax(5.5rem,1fr));gap:.5rem}vz-booking button{font:inherit;cursor:pointer;min-height:44px;padding:.5rem .875rem;border:1px solid var(--color-line,#d4d4d8);border-radius:var(--radius-card,.75rem);background:transparent;color:inherit;white-space:nowrap}vz-booking button[aria-pressed=true],vz-booking [data-vz-submit]{background:var(--color-brand,#18181b);border-color:var(--color-brand,#18181b);color:var(--color-brand-ink,#fff)}vz-booking button:focus-visible,vz-booking input:focus-visible,vz-booking textarea:focus-visible{outline:2px solid var(--color-brand,#18181b);outline-offset:2px}vz-booking button:disabled{opacity:.6;cursor:wait}vz-booking form{display:grid;gap:.75rem}vz-booking form[hidden]{display:none}vz-booking label{display:grid;gap:.25rem}vz-booking [data-vz-consent-label]{display:flex;gap:.5rem;align-items:flex-start}vz-booking input:not([type=checkbox]),vz-booking textarea{font:inherit;width:100%;box-sizing:border-box;padding:.625rem .75rem;border:1px solid var(--color-line,#d4d4d8);border-radius:calc(var(--radius-card,.75rem)*.6);background:var(--color-surface,transparent);color:inherit}vz-booking [aria-invalid=true]{border-color:#dc2626}vz-booking [data-vz-muted]{margin:0;font-size:.875em;opacity:.75}vz-booking [data-vz-actions]{display:flex;flex-wrap:wrap;gap:.5rem}vz-booking [data-vz-change]{background:transparent;color:inherit}vz-booking a{color:var(--color-brand,inherit);font-weight:600}@media (prefers-reduced-motion:no-preference){vz-booking button{transition:background-color .15s,border-color .15s}}";

const MESSAGE_KEYS = ["sending","success","failed","invalid","rateLimited","unavailable","consentTitle","consentText","acceptAll","rejectAll","customize","save","necessary","analytics","marketing","privacy","bookingLoading","bookingEmpty","bookingUnavailable","bookingDays","bookingTimes","bookingZone","bookingBusinessZone","bookingName","bookingPhone","bookingEmail","bookingMessage","bookingSubmit","bookingTaken","bookingDone","bookingMeet","bookingPreview","bookingChange","bookingChosen"];

const MESSAGES = {
  pt: "Enviando...|Recebemos seus dados. Obrigado!|Não foi possível enviar. Tente de novo.|Confira os campos destacados.|Muitas tentativas. Aguarde um instante.|Formulário indisponível no momento.|Sua privacidade|Usamos cookies necessários e, se você permitir, de análise e de marketing.|Aceitar todos|Recusar opcionais|Personalizar|Salvar escolhas|Necessários|Análise|Marketing|Política de privacidade|Carregando horários...|Nenhum horário livre nos próximos dias.|Agenda indisponível no momento.|Dias disponíveis|Horários disponíveis|Horários no seu fuso ({zone}).|Atendimento no fuso {zone}.|Nome|WhatsApp|E-mail|Mensagem (opcional)|Agendar|Esse horário acabou de ser reservado. Escolha outro.|Agendado para {when}.|Entrar na reunião|Prévia: horários de exemplo, nada é reservado.|Trocar horário|Horário escolhido: {when}",
  en: "Sending...|We got your details. Thank you!|Could not send. Please try again.|Please check the highlighted fields.|Too many attempts. Please wait a moment.|This form is unavailable right now.|Your privacy|We use necessary cookies and, if you allow, analytics and marketing cookies.|Accept all|Reject optional|Customize|Save choices|Necessary|Analytics|Marketing|Privacy policy|Loading times...|No free times in the next days.|Scheduling is unavailable right now.|Available days|Available times|Times in your time zone ({zone}).|Business time zone: {zone}.|Name|WhatsApp|Email|Message (optional)|Book|That time was just taken. Please pick another.|Booked for {when}.|Join the meeting|Preview: sample times, nothing is booked.|Change time|Chosen time: {when}",
  es: "Enviando...|Recibimos tus datos. ¡Gracias!|No fue posible enviar. Inténtalo de nuevo.|Revisa los campos destacados.|Demasiados intentos. Espera un momento.|Formulario no disponible ahora.|Tu privacidad|Usamos cookies necesarias y, si lo permites, de análisis y de marketing.|Aceptar todas|Rechazar opcionales|Personalizar|Guardar elección|Necesarias|Análisis|Marketing|Política de privacidad|Cargando horarios...|No hay horarios libres en los próximos días.|Agenda no disponible ahora.|Días disponibles|Horarios disponibles|Horarios en tu zona horaria ({zone}).|Atención en la zona horaria {zone}.|Nombre|WhatsApp|Correo|Mensaje (opcional)|Agendar|Ese horario acaba de reservarse. Elige otro.|Agendado para {when}.|Entrar a la reunión|Vista previa: horarios de ejemplo, no se reserva nada.|Cambiar horario|Horario elegido: {when}",
  de: "Wird gesendet...|Wir haben Ihre Angaben erhalten. Danke!|Senden fehlgeschlagen. Bitte erneut versuchen.|Bitte prüfen Sie die markierten Felder.|Zu viele Versuche. Bitte kurz warten.|Formular derzeit nicht verfügbar.|Ihre Privatsphäre|Wir nutzen notwendige Cookies und, mit Ihrer Zustimmung, Analyse- und Marketing-Cookies.|Alle akzeptieren|Optionale ablehnen|Anpassen|Auswahl speichern|Notwendig|Analyse|Marketing|Datenschutzerklärung|Termine werden geladen...|In den nächsten Tagen ist kein Termin frei.|Terminbuchung derzeit nicht verfügbar.|Verfügbare Tage|Verfügbare Uhrzeiten|Uhrzeiten in Ihrer Zeitzone ({zone}).|Zeitzone des Anbieters: {zone}.|Name|WhatsApp|E-Mail|Nachricht (optional)|Buchen|Dieser Termin wurde gerade vergeben. Bitte wählen Sie einen anderen.|Gebucht für {when}.|Zum Meeting|Vorschau: Beispieltermine, es wird nichts gebucht.|Termin ändern|Gewählter Termin: {when}",
};

export function messagesFor(lang) {
  const words = (MESSAGES[String(lang || "").slice(0, 2).toLowerCase()] || MESSAGES.pt).split("|");
  return Object.fromEntries(MESSAGE_KEYS.map((key, i) => [key, words[i]]));
}

function cookieValue(cookie, name) {
  for (const part of String(cookie || "").split(";")) {
    const at = part.indexOf("=");
    if (at > 0 && part.slice(0, at).trim() === name) {
      try {
        return decodeURIComponent(part.slice(at + 1).trim());
      } catch {
        return "";
      }
    }
  }
  return "";
}

export function attribution({ search, cookie, referrer, now }) {
  const query = new URLSearchParams(search || "");
  const utm = {};
  for (const key of UTM) utm[key] = query.get("utm_" + key) || "";
  const fbclid = query.get("fbclid") || "";
  return {
    utm,
    referrer: referrer || "",
    fbclid,
    gclid: query.get("gclid") || "",
    fbp: cookieValue(cookie, "_fbp"),
    fbc: cookieValue(cookie, "_fbc") || (fbclid ? "fb.1." + now + "." + fbclid : ""),
    variant: cookieValue(cookie, "vz_variant"),
  };
}

export function formFields(entries) {
  const fields = {};
  for (const [name, value] of entries) {
    if (name === TRAP_NAME || typeof value !== "string") continue;
    if (!Object.hasOwn(fields, name)) fields[name] = value;
    else fields[name] = [].concat(fields[name], value);
  }
  return fields;
}

function isApiOrigin(value) {
  try {
    const url = new URL(value);
    return url.origin === value && (url.protocol === "https:" || url.hostname === "localhost" || url.hostname === "127.0.0.1");
  } catch {
    return false;
  }
}

function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function postJson(fetchImpl, url, body) {
  const response = await fetchImpl(url, { method: "POST", headers: { "Content-Type": "application/json" }, credentials: "omit", body: JSON.stringify(body) });
  let json = null;
  try {
    json = await response.json();
  } catch {
    json = null;
  }
  return { status: response.status, ok: response.ok, json };
}

export function successAction(value) {
  if (!isRecord(value)) return { kind: "message" };
  if (value.kind === "redirect" && typeof value.route === "string" && LOCAL_ROUTE.test(value.route)) return { kind: "redirect", route: value.route };
  if (value.kind === "whatsapp" && typeof value.url === "string" && /^https:\/\//.test(value.url)) return { kind: "whatsapp", url: value.url };
  if (value.kind === "message" && typeof value.message === "string" && value.message.trim()) return { kind: "message", message: value.message.slice(0, 500) };
  return { kind: "message" };
}

export function webchatLoader(api, key) {
  return isApiOrigin(api) && WEBCHAT_KEY.test(key || "") ? api + WEBCHAT_LOADER : "";
}

export async function fetchProof({ api, formKey, fetch: fetchImpl, solve, now }) {
  const reply = await postJson(fetchImpl, api + "/public/sites/forms/" + formKey + "/challenge", {});
  if (!reply.ok || !isRecord(reply.json)) throw new Error("challenge refused");
  const fetchedAt = now();
  return { challenge: reply.json, nonce: await solve(reply.json), fetchedAt };
}

const refusal = (reason, fields = []) => ({ ok: false, reason, fields });

function bookedOf(value) {
  if (!isRecord(value) || !SLOT_TIME.test(value.start || "")) return null;
  return {
    start: value.start,
    end: SLOT_TIME.test(value.end || "") ? value.end : "",
    timeZone: typeof value.timeZone === "string" ? value.timeZone : "",
    meetingLink: typeof value.meetingLink === "string" && /^https:\/\//.test(value.meetingLink) ? value.meetingLink : "",
  };
}

export async function fetchSlots({ api, formKey, fetch: fetchImpl }) {
  if (!isApiOrigin(api) || !FORM_KEY.test(formKey || "")) return null;
  try {
    const response = await fetchImpl(api + "/public/sites/forms/" + formKey + "/slots", { method: "GET", credentials: "omit" });
    if (!response.ok) return null;
    const json = await response.json();
    if (!isRecord(json) || !Array.isArray(json.slots) || typeof json.timeZone !== "string") return null;
    return {
      timeZone: json.timeZone,
      durationMinutes: Number(json.durationMinutes) || 0,
      fields: Array.isArray(json.fields) ? json.fields.filter((field) => BOOKING_FIELDS.includes(field)) : [],
      consentText: typeof json.consentText === "string" ? json.consentText.slice(0, 2000) : "",
      preview: json.preview === true,
      slots: json.slots.filter((slot) => isRecord(slot) && SLOT_TIME.test(slot.start || "") && SLOT_TIME.test(slot.end || "")),
    };
  } catch {
    return null;
  }
}

export function groupSlots(slots, timeZone, locale) {
  const dayKey = new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" });
  const dayLabel = new Intl.DateTimeFormat(locale || "pt-BR", { timeZone, weekday: "short", day: "numeric", month: "short" });
  const timeLabel = new Intl.DateTimeFormat(locale || "pt-BR", { timeZone, hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  const days = [];
  for (const slot of slots) {
    const at = new Date(slot.start);
    const key = dayKey.format(at);
    let day = days.find((candidate) => candidate.key === key);
    if (!day) {
      day = { key, label: dayLabel.format(at), times: [] };
      days.push(day);
    }
    day.times.push({ start: slot.start, end: slot.end, label: timeLabel.format(at) });
  }
  return days;
}

export async function sendSubmission(run) {
  const { api, formKey, body, now, wait } = run;
  const booking = run.path === "bookings";
  if (!isApiOrigin(api) || !FORM_KEY.test(formKey || "")) return refusal("unavailable");
  let proof = run.proof && now() - run.proof.fetchedAt < PROOF_REUSE_MS ? run.proof : null;
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      proof = proof || (await fetchProof(run));
      const pause = proof.fetchedAt + MIN_FILL_MS - now();
      if (pause > 0) await wait(pause);
      const reply = await postJson(run.fetch, api + "/public/sites/forms/" + formKey + (booking ? "/bookings" : "/submissions"), { ...body, challengeToken: proof.challenge.token, nonce: proof.nonce });
      const code = isRecord(reply.json) && typeof reply.json.code === "string" ? reply.json.code : "";
      if (reply.ok) {
        if (!isRecord(reply.json)) return refusal("failed");
        const done = { ok: true, id: typeof reply.json.id === "string" ? reply.json.id : "", success: successAction(reply.json.success) };
        return booking ? { ...done, booking: bookedOf(reply.json.booking) } : done;
      }
      if (code.startsWith("challenge_")) {
        if (attempt > 0) return refusal("failed");
        proof = null;
        continue;
      }
      if (reply.status === 429) return refusal("rate_limited");
      if (reply.status === 409) return refusal("taken");
      if (reply.status === 503) return refusal("unavailable");
      if (reply.status === 400 || reply.status === 422) return refusal("invalid", isRecord(reply.json) && isRecord(reply.json.details) ? Object.keys(reply.json.details) : []);
      return refusal("failed");
    } catch {
      return refusal("failed");
    }
  }
  return refusal("failed");
}

export function readConsent(storage) {
  try {
    const value = JSON.parse(storage.getItem(CONSENT_KEY));
    if (isRecord(value) && value.v === 1 && typeof value.analytics === "boolean" && typeof value.marketing === "boolean") return { analytics: value.analytics, marketing: value.marketing };
  } catch {
    return null;
  }
  return null;
}

export function writeConsent(storage, choice, now) {
  try {
    storage.setItem(CONSENT_KEY, JSON.stringify({ v: 1, analytics: choice.analytics, marketing: choice.marketing, at: new Date(now).toISOString() }));
  } catch {
    return;
  }
}

export function consentSignals(choice) {
  const state = (granted) => (granted ? "granted" : "denied");
  const marketing = state(!!(choice && choice.marketing));
  return { analytics_storage: state(!!(choice && choice.analytics)), ad_storage: marketing, ad_user_data: marketing, ad_personalization: marketing };
}

export function popupDue(lastShown, now, frequencyDays) {
  const last = Number(lastShown);
  if (lastShown === null || !Number.isFinite(last) || frequencyDays <= 0) return true;
  return now - last >= frequencyDays * DAY_MS;
}

export function countdownParts(deadline, now) {
  const left = Math.max(0, Math.floor((deadline - now) / 1000));
  return { days: Math.floor(left / 86400), hours: Math.floor((left % 86400) / 3600), minutes: Math.floor((left % 3600) / 60), seconds: left % 60, done: left === 0 };
}

export function evergreenDeadline(storage, key, minutes, now) {
  try {
    const stored = Number(storage.getItem(key));
    if (stored > 0) return stored;
    storage.setItem(key, String(now + minutes * 60000));
  } catch {
    return now + minutes * 60000;
  }
  return now + minutes * 60000;
}

const get = (el, name) => el.getAttribute(name);
const originOf = (url) => {
  try {
    return new URL(url).origin;
  } catch {
    return "";
  }
};
const set = (el, name, value) => el.setAttribute(name, value);
const unset = (el, name) => el.removeAttribute(name);
const one = (el, selector) => el.querySelector(selector);
const all = (el, selector) => el.querySelectorAll(selector);
const on = (target, type, listener, options) => target.addEventListener(type, listener, options);

export function startRuntime(win, deps = {}) {
  const now = deps.now || Date.now;
  const wait = deps.wait || ((ms) => new Promise((done) => win.setTimeout(done, ms)));
  const go = deps.go || ((url) => win.location.assign(url));
  const fetchImpl = deps.fetch || win.fetch.bind(win);
  const solve = deps.solve || solveChallenge;
  const doc = win.document;
  const root = doc.documentElement;
  const text = messagesFor(root.lang);
  const boxOf = (el) => el[VZ] || (el[VZ] = {});
  const fresh = (el) => !boxOf(el).attached && (boxOf(el).attached = true);
  const storage = (() => {
    try {
      return win.localStorage;
    } catch {
      return { getItem: () => null, setItem: () => {} };
    }
  })();
  const reducedMotion = () => Boolean(win.matchMedia && win.matchMedia("(prefers-reduced-motion: reduce)").matches);
  const opt = (el, name) => get(el, name) ?? get(el, "data-" + name);
  const forms = (() => {
    try {
      const map = JSON.parse(get(root, "data-vz-forms") || "{}");
      return isRecord(map) ? map : {};
    } catch {
      return {};
    }
  })();
  const found = attribution({ search: win.location.search, cookie: doc.cookie, referrer: doc.referrer, now: now() });
  const route = get(root, "data-vz-route") || win.location.pathname;
  const variant = get(root, "data-vz-variant") || found.variant;

  const signal = (type, detail) => {
    try {
      win.dispatchEvent(new win.CustomEvent(type, { detail }));
    } catch {
      return;
    }
  };
  const preview = root.hasAttribute(PREVIEW_ATTR);
  const beacon = (event, extra) => {
    if (preview) return;
    try {
      const payload = JSON.stringify({ event, route, variant, ...extra });
      win.navigator.sendBeacon(BEACON_PATH, new Blob([payload], { type: "application/json" }));
    } catch {
      return;
    }
  };

  function gtag() {
    win.dataLayer = win.dataLayer || [];
    win.dataLayer.push(arguments);
  }
  const applyConsent = (choice) => {
    gtag("consent", "update", consentSignals(choice));
    for (const script of all(doc, 'script[type="text/plain"][data-vz-consent]')) {
      if (!choice[get(script, "data-vz-consent")]) continue;
      const live = doc.createElement("script");
      for (const attribute of script.attributes) {
        if (attribute.name !== "type" && attribute.name !== "data-vz-consent") set(live, attribute.name, attribute.value);
      }
      live.text = script.text;
      script.replaceWith(live);
    }
  };

  const formOf = (form) => form.closest("vz-form") || form;
  const stateOf = (form) => {
    const box = boxOf(form);
    return box.form || (box.form = { idempotencyKey: "", proof: null, preparing: null, started: false });
  };
  const statusOf = (form) => {
    let status = one(form, "[data-vz-status]");
    if (!status) {
      status = doc.createElement("p");
      set(status, "data-vz-status", "");
      form.appendChild(status);
    }
    set(status, "aria-live", "polite");
    return status;
  };
  const keyOf = (form) => {
    const owner = formOf(form);
    const id = owner.tagName === "VZ-FORM" ? owner.id : get(form, "data-vz-form");
    return get(owner, "data-vz-form-key") || get(form, "data-vz-form-key") || (typeof forms[id] === "string" ? forms[id] : "");
  };
  const identity = (form) => formOf(form).id || get(form, "data-vz-form") || "";
  const runInput = (form) => ({ api: api(), formKey: keyOf(form), fetch: fetchImpl, solve, now, wait });

  const attachForm = (form) => {
    if (!fresh(form)) return;
    if (!one(form, '[name="' + TRAP_NAME + '"]')) {
      const trap = doc.createElement("div");
      set(trap, "aria-hidden", "true");
      trap.style.cssText = "position:absolute;left:-1e4px";
      trap.innerHTML = '<input type="text" name="' + TRAP_NAME + '" tabindex="-1" autocomplete="off">';
      form.appendChild(trap);
    }
    const watcher = new win.IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      watcher.disconnect();
      beacon("form_view", { form: identity(form) });
    }, { threshold: 0.5 });
    watcher.observe(form);
  };
  const startForm = (form) => {
    const state = stateOf(form);
    if (state.started) return;
    state.started = true;
    beacon("form_start", { form: identity(form) });
    const input = runInput(form);
    if (!isApiOrigin(input.api) || !FORM_KEY.test(input.formKey)) return;
    state.preparing = fetchProof(input).then((proof) => {
      state.proof = proof;
    }, () => {
      state.proof = null;
    });
  };
  const submitForm = async (form) => {
    const state = stateOf(form);
    if (get(form, "aria-busy") === "true") return;
    const status = statusOf(form);
    const button = one(form, '[type="submit"],button:not([type])');
    for (const field of all(form, '[aria-invalid="true"]')) unset(field, "aria-invalid");
    set(form, "aria-busy", "true");
    if (button) button.disabled = true;
    unset(status, "role");
    status.textContent = text.sending;
    startForm(form);
    if (state.preparing) await state.preparing;
    state.idempotencyKey = state.idempotencyKey || win.crypto.randomUUID();
    const data = new win.FormData(form);
    const body = { ...found, idempotencyKey: state.idempotencyKey, website: String(data.get(TRAP_NAME) || ""), route, variant, fields: formFields(data.entries()) };
    const outcome = await sendSubmission({ ...runInput(form), body, proof: state.proof });
    state.proof = null;
    state.preparing = null;
    unset(form, "aria-busy");
    if (button) button.disabled = false;
    if (!outcome.ok) {
      const invalid = outcome.fields.map((name) => one(form, '[name="' + name.replace(/["\\]/g, "") + '"]')).filter(Boolean);
      for (const field of invalid) set(field, "aria-invalid", "true");
      set(status, "role", "alert");
      status.textContent = { invalid: text.invalid, rate_limited: text.rateLimited, unavailable: text.unavailable }[outcome.reason] || text.failed;
      if (invalid[0]) invalid[0].focus();
      return;
    }
    state.idempotencyKey = "";
    beacon("form_submit", { form: identity(form) });
    signal("vz:lead", outcome.id ? { eventId: outcome.id, form: identity(form) } : { form: identity(form) });
    const action = outcome.success;
    if (action.kind === "redirect") return go(action.route);
    if (action.kind === "whatsapp") {
      beacon("whatsapp", { target: action.url });
      signal("vz:contact", { target: action.url });
      return go(action.url);
    }
    form.reset();
    status.textContent = action.message || text.success;
    status.scrollIntoView({ block: "nearest", behavior: reducedMotion() ? "auto" : "smooth" });
  };

  let returnFocus = null;
  const hostOf = (el) => el.closest("vz-popup") || el;
  const popupKey = (el) => "vz-popup:" + (el.id || opt(hostOf(el), "popup-id") || route);
  const restoreFocus = () => {
    if (returnFocus && returnFocus.focus) returnFocus.focus();
    returnFocus = null;
  };
  const closePopup = (el) => {
    el.close();
    restoreFocus();
  };
  const showPopup = (el) => {
    if (el.open || !el.isConnected || one(doc, "dialog[open]")) return;
    try {
      if (!popupDue(storage.getItem(popupKey(el)), now(), Number(opt(hostOf(el), "frequency") ?? 7))) return;
      storage.setItem(popupKey(el), String(now()));
    } catch {
      return;
    }
    returnFocus = doc.activeElement;
    el.showModal();
    (one(el, FOCUSABLE) || el).focus();
  };
  const attachPopup = (el) => {
    if (!fresh(el)) return;
    const host = hostOf(el);
    on(el, "cancel", (event) => {
      event.preventDefault();
      closePopup(el);
    });
    on(el, "close", restoreFocus);
    const trigger = opt(host, "trigger") || get(el, "data-vz-popup") || "timer";
    if (trigger === "exit") {
      on(doc, "mouseout", (event) => {
        if (!event.relatedTarget && event.clientY <= 0) showPopup(el);
      });
    } else if (trigger === "scroll") {
      const depth = Number(opt(host, "depth") || 50) / 100;
      const onScroll = () => {
        const reach = (win.scrollY + win.innerHeight) / Math.max(1, root.scrollHeight);
        if (reach < depth) return;
        win.removeEventListener("scroll", onScroll);
        showPopup(el);
      };
      on(win, "scroll", onScroll, { passive: true });
    } else {
      win.setTimeout(() => showPopup(el), Number(opt(host, "delay") || 5) * 1000);
    }
  };

  const attachCountdown = (el) => {
    if (!fresh(el)) return;
    set(el, "role", "timer");
    const minutes = Number(opt(el, "evergreen"));
    const deadline = minutes > 0 ? evergreenDeadline(storage, "vz-countdown:" + (el.id || route), minutes, now()) : Date.parse(opt(el, "deadline") || "");
    if (!Number.isFinite(deadline)) return;
    const pad = (value) => String(value).padStart(2, "0");
    let timer = 0;
    const tick = () => {
      if (!el.isConnected) return win.clearInterval(timer);
      const parts = countdownParts(deadline, now());
      const units = all(el, "[data-unit]");
      if (units.length) for (const unit of units) unit.textContent = pad(parts[get(unit, "data-unit")] ?? 0);
      else el.textContent = (parts.days ? parts.days + "d " : "") + pad(parts.hours) + ":" + pad(parts.minutes) + ":" + pad(parts.seconds);
      if (!parts.done) return;
      win.clearInterval(timer);
      const expired = opt(el, "expired") || "";
      const target = opt(el, "expired-route") || "";
      if (expired === "hide") el.hidden = true;
      if (expired === "redirect" && LOCAL_ROUTE.test(target) && target !== win.location.pathname) go(target);
    };
    timer = win.setInterval(tick, 1000);
    tick();
  };

  const consentPanel = (el) => {
    const privacy = get(el, "privacy");
    set(el, "role", "region");
    set(el, "aria-label", text.consentTitle);
    if (!el.className) el.style.cssText = "position:fixed;inset:auto 16px 16px;z-index:9999;max-width:40rem;margin:auto;padding:16px;border-radius:12px;background:#fff;color:#111;box-shadow:0 8px 32px #0003";
    el.innerHTML = "";
    el.hidden = false;
    const add = (parent, tag, content, attrs) => {
      const node = doc.createElement(tag);
      if (content) node.textContent = content;
      for (const [name, value] of Object.entries(attrs || {})) set(node, name, value);
      parent.appendChild(node);
      return node;
    };
    add(el, "strong", get(el, "heading") || text.consentTitle);
    const para = add(el, "p", get(el, "text") || text.consentText);
    if (privacy && /^(https:\/\/|\/(?!\/))/.test(privacy)) {
      para.append(" ");
      add(para, "a", text.privacy, { href: privacy });
    }
    const choices = add(el, "fieldset", "", { hidden: "" });
    const box = (name, checked, disabled) => {
      const label = add(choices, "label");
      const input = add(label, "input", "", { type: "checkbox", name });
      input.checked = checked;
      input.disabled = disabled;
      label.append(" " + text[name]);
      return input;
    };
    const stored = readConsent(storage) || { analytics: false, marketing: false };
    box("necessary", true, true);
    const analytics = box("analytics", stored.analytics, false);
    const marketing = box("marketing", stored.marketing, false);
    const actions = add(el, "div");
    const decide = (choice) => {
      writeConsent(storage, choice, now());
      applyConsent(choice);
      el.hidden = true;
    };
    on(add(actions, "button", text.rejectAll, { type: "button" }), "click", () => decide({ analytics: false, marketing: false }));
    on(add(actions, "button", text.acceptAll, { type: "button" }), "click", () => decide({ analytics: true, marketing: true }));
    const custom = add(actions, "button", text.customize, { type: "button" });
    on(custom, "click", () => {
      if (choices.hidden) {
        choices.hidden = false;
        custom.textContent = text.save;
        analytics.focus();
      } else decide({ analytics: analytics.checked, marketing: marketing.checked });
    });
  };
  const attachConsent = (el) => {
    if (!fresh(el)) return;
    gtag("consent", "default", { ...consentSignals(null), wait_for_update: 500 });
    const choice = readConsent(storage);
    if (choice) {
      el.hidden = true;
      applyConsent(choice);
    } else consentPanel(el);
  };

  const api = () => (get(root, "data-vz-api") || "").replace(/\/$/, "");
  const attachWebchat = (el) => {
    if (!fresh(el)) return;
    const key = get(el, "data-vz-webchat-key");
    const src = webchatLoader(api(), key);
    if (!src || one(doc, "script[data-vz-webchat]")) return;
    const script = doc.createElement("script");
    script.async = true;
    set(script, "src", src);
    set(script, "data-key", key);
    set(script, "data-vz-webchat", "");
    doc.body.appendChild(script);
  };

  const fill = (template, values) => template.replace(/\{(\w+)\}/g, (_, key) => values[key] ?? "");
  const visitorZone = () => {
    if (deps.timeZone) return deps.timeZone;
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
    } catch {
      return "UTC";
    }
  };
  const make = (tag, attrs, content) => {
    const node = doc.createElement(tag);
    for (const [name, value] of Object.entries(attrs || {})) {
      if (value === true) set(node, name, "");
      else if (value !== false && value !== null && value !== undefined) set(node, name, String(value));
    }
    if (content) node.textContent = content;
    return node;
  };
  const bookingStyle = () => {
    if (one(doc, "style[data-vz-booking-style]")) return;
    const style = make("style", { "data-vz-booking-style": true });
    style.textContent = BOOKING_CSS;
    (doc.head || root).appendChild(style);
  };
  const typedValues = (form) => {
    const out = {};
    if (!form) return out;
    for (const field of all(form, "input[name],textarea[name]")) {
      if (field.type !== "checkbox" && field.name !== TRAP_NAME) out[field.name] = field.value;
    }
    return out;
  };
  const attachBooking = (el) => {
    if (!fresh(el)) return;
    const id = get(el, "booking") || "";
    const formKey = get(el, "data-vz-form-key") || (typeof forms[id] === "string" ? forms[id] : "");
    const locale = root.lang || "pt-BR";
    const zone = visitorZone();
    const when = (start) => {
      try {
        return new Intl.DateTimeFormat(locale, { timeZone: zone, weekday: "long", day: "numeric", month: "long", hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(new Date(start));
      } catch {
        return start;
      }
    };
    if (!el.hasAttribute("unstyled")) bookingStyle();
    el.textContent = "";
    const shell = make("div", { "data-vz-booking-root": true });
    const status = make("p", { "data-vz-status": true, "aria-live": "polite" });
    el.append(shell, status);
    const state = { idempotencyKey: "", proof: null, preparing: null, started: false, chosen: null };
    let form = null;
    const unavailable = () => {
      unset(el, "aria-busy");
      shell.textContent = "";
      form = null;
      status.textContent = text.bookingUnavailable;
    };
    if (!isApiOrigin(api()) || !FORM_KEY.test(formKey)) return unavailable();
    set(el, "aria-busy", "true");
    status.textContent = text.bookingLoading;
    const watcher = new win.IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return;
      watcher.disconnect();
      beacon("form_view", { form: id });
    }, { threshold: 0.5 });
    watcher.observe(el);
    const prepare = () => {
      if (state.started) return;
      state.started = true;
      beacon("form_start", { form: id });
      state.preparing = fetchProof({ api: api(), formKey, fetch: fetchImpl, solve, now }).then((proof) => {
        state.proof = proof;
      }, () => {
        state.proof = null;
      });
    };
    const buildForm = (data, typed) => {
      const next = make("form", { "data-vz-booking-form": true });
      next.hidden = true;
      next.append(make("p", { "data-vz-chosen": true }));
      const field = (name, label, attrs, tag = "input") => {
        const wrap = make("label");
        wrap.append(make("span", null, label));
        const input = make(tag, { ...attrs, name });
        if (typed[name]) input.value = typed[name];
        wrap.append(input);
        next.append(wrap);
      };
      field("lead.name", text.bookingName, { type: "text", required: true, autocomplete: "name", maxlength: 120 });
      field("lead.phone", text.bookingPhone, { type: "tel", required: true, autocomplete: "tel", inputmode: "tel" });
      if (data.fields.includes("email")) field("lead.email", text.bookingEmail, { type: "email", autocomplete: "email" });
      if (data.fields.includes("message")) field("field.mensagem", text.bookingMessage, { maxlength: 1000, rows: 3 }, "textarea");
      const consent = make("label", { "data-vz-consent-label": true });
      consent.append(make("input", { type: "checkbox", name: "consent", required: true }), make("span", null, data.consentText));
      next.append(consent);
      const actions = make("div", { "data-vz-actions": true });
      const change = make("button", { type: "button", "data-vz-change": true }, text.bookingChange);
      actions.append(make("button", { type: "submit", "data-vz-submit": true }, text.bookingSubmit), change);
      next.append(actions);
      const trap = make("div", { "aria-hidden": "true" });
      trap.style.cssText = "position:absolute;left:-1e4px";
      trap.append(make("input", { type: "text", name: TRAP_NAME, tabindex: "-1", autocomplete: "off" }));
      next.append(trap);
      on(change, "click", () => {
        state.chosen = null;
        next.hidden = true;
        for (const button of all(shell, "[data-vz-times] button")) set(button, "aria-pressed", "false");
        const first = one(shell, "[data-vz-times] button");
        if (first) first.focus();
      });
      on(next, "submit", (event) => {
        event.preventDefault();
        event.stopPropagation();
        submit();
      });
      return next;
    };
    const render = (data, message = "") => {
      unset(el, "aria-busy");
      const typed = typedValues(form);
      shell.textContent = "";
      unset(status, "role");
      status.textContent = message;
      if (message) set(status, "role", "alert");
      if (data.preview) shell.append(make("p", { "data-vz-muted": true }, text.bookingPreview));
      const zoneLine = fill(text.bookingZone, { zone }) + (data.timeZone && data.timeZone !== zone ? " " + fill(text.bookingBusinessZone, { zone: data.timeZone }) : "");
      shell.append(make("p", { "data-vz-muted": true, "data-vz-booking-zone": true }, zoneLine));
      let days = [];
      try {
        days = groupSlots(data.slots, zone, locale);
      } catch {
        days = [];
      }
      if (!days.length) {
        form = null;
        status.textContent = message || text.bookingEmpty;
        return;
      }
      const dayRow = make("div", { role: "group", "aria-label": text.bookingDays, "data-vz-days": true });
      const timeRow = make("div", { role: "group", "aria-label": text.bookingTimes, "data-vz-times": true });
      form = buildForm(data, typed);
      shell.append(dayRow, timeRow, form);
      const choose = (time, button) => {
        state.chosen = time;
        for (const other of all(timeRow, "button")) set(other, "aria-pressed", String(other === button));
        form.hidden = false;
        one(form, "[data-vz-chosen]").textContent = fill(text.bookingChosen, { when: when(time.start) });
        prepare();
        const first = one(form, 'input[name="lead.name"]');
        if (first) first.focus();
      };
      const showDay = (index) => {
        for (const [at, button] of [...dayRow.children].entries()) set(button, "aria-pressed", String(at === index));
        timeRow.textContent = "";
        for (const time of days[index].times) {
          const button = make("button", { type: "button", "aria-pressed": String(Boolean(state.chosen && state.chosen.start === time.start)) }, time.label);
          on(button, "click", () => choose(time, button));
          timeRow.append(button);
        }
      };
      days.forEach((day, at) => {
        const button = make("button", { type: "button", "aria-pressed": "false" }, day.label);
        on(button, "click", () => showDay(at));
        dayRow.append(button);
      });
      const chosenDay = state.chosen ? days.findIndex((day) => day.times.some((time) => time.start === state.chosen.start)) : -1;
      if (chosenDay < 0) state.chosen = null;
      showDay(Math.max(0, chosenDay));
      if (state.chosen) {
        form.hidden = false;
        one(form, "[data-vz-chosen]").textContent = fill(text.bookingChosen, { when: when(state.chosen.start) });
      }
    };
    const load = async (message) => {
      const data = await fetchSlots({ api: api(), formKey, fetch: fetchImpl });
      if (!data) return unavailable();
      render(data, message);
    };
    const submit = async () => {
      if (!form || get(form, "aria-busy") === "true" || !state.chosen) return;
      const current = form;
      const chosen = state.chosen;
      const button = one(current, "[data-vz-submit]");
      for (const field of all(current, '[aria-invalid="true"]')) unset(field, "aria-invalid");
      set(current, "aria-busy", "true");
      if (button) button.disabled = true;
      unset(status, "role");
      status.textContent = text.sending;
      prepare();
      if (state.preparing) await state.preparing;
      state.idempotencyKey = state.idempotencyKey || win.crypto.randomUUID();
      const data = new win.FormData(current);
      const body = { ...found, idempotencyKey: state.idempotencyKey, website: String(data.get(TRAP_NAME) || ""), route, variant, fields: { ...formFields(data.entries()), [SLOT_FIELD]: chosen.start } };
      const outcome = await sendSubmission({ api: api(), formKey, path: "bookings", fetch: fetchImpl, solve, now, wait, body, proof: state.proof });
      state.proof = null;
      state.preparing = null;
      state.started = false;
      unset(current, "aria-busy");
      if (button) button.disabled = false;
      if (!outcome.ok) {
        if (outcome.reason === "taken") {
          state.idempotencyKey = "";
          state.chosen = null;
          return load(text.bookingTaken);
        }
        const invalid = outcome.fields.map((name) => one(current, '[name="' + name.replace(/["\\]/g, "") + '"]')).filter(Boolean);
        for (const field of invalid) set(field, "aria-invalid", "true");
        set(status, "role", "alert");
        status.textContent = { invalid: text.invalid, rate_limited: text.rateLimited, unavailable: text.bookingUnavailable }[outcome.reason] || text.failed;
        if (invalid[0]) invalid[0].focus();
        return;
      }
      state.idempotencyKey = "";
      beacon("form_submit", { form: id });
      signal("vz:lead", outcome.id ? { eventId: outcome.id, form: id } : { form: id });
      const action = outcome.success;
      if (action.kind === "redirect") return go(action.route);
      if (action.kind === "whatsapp") {
        beacon("whatsapp", { target: action.url });
        signal("vz:contact", { target: action.url });
        return go(action.url);
      }
      shell.textContent = "";
      form = null;
      const booked = outcome.booking;
      status.textContent = (action.message ? action.message + " " : "") + fill(text.bookingDone, { when: when(booked ? booked.start : chosen.start) });
      if (booked && booked.meetingLink) shell.append(make("a", { href: booked.meetingLink, target: "_blank", rel: "noopener" }, text.bookingMeet));
      status.scrollIntoView({ block: "nearest", behavior: reducedMotion() ? "auto" : "smooth" });
    };
    load("");
  };

  const attachers = [
    ["vz-form form, form[data-vz-form]", attachForm],
    ["vz-popup > dialog, dialog[data-vz-popup]", attachPopup],
    ["vz-countdown, [data-vz-countdown]", attachCountdown],
    ["vz-consent", attachConsent],
    ["vz-webchat", attachWebchat],
    ["vz-booking", attachBooking],
  ];
  const scan = (node) => {
    if (node.nodeType !== 1) return;
    for (const [selector, attach] of attachers) {
      if (node.matches(selector)) attach(node);
      for (const el of all(node, selector)) attach(el);
    }
    for (const link of all(node, "vz-whatsapp[href]")) {
      if (one(link, "a")) continue;
      set(link, "role", "link");
      link.tabIndex = 0;
    }
  };

  const formTarget = (target) => {
    const form = target && target.closest ? target.closest("form") : null;
    return form && (form.hasAttribute("data-vz-form") || form.closest("vz-form")) ? form : null;
  };
  on(doc, "submit", (event) => {
    const form = formTarget(event.target);
    if (!form) return;
    event.preventDefault();
    event.stopPropagation();
    submitForm(form);
  }, true);
  on(doc, "focusin", (event) => {
    const form = formTarget(event.target);
    if (form) startForm(form);
  });
  const whatsapp = (target) => target.closest("a[data-vz-whatsapp], vz-whatsapp a, vz-whatsapp[href]");
  const follow = (link) => {
    const href = get(link, "href") || "";
    beacon("whatsapp", { target: href });
    signal("vz:contact", { target: href });
    if (link.tagName === "VZ-WHATSAPP" && /^https:\/\//.test(href)) win.open(href, "_blank", "noopener");
  };
  on(doc, "click", (event) => {
    const target = event.target;
    if (!target || !target.closest) return;
    const link = whatsapp(target);
    if (link) follow(link);
    const cta = target.closest("[data-vz-cta]");
    if (cta) beacon("cta", { target: get(cta, "data-vz-cta") || get(cta, "href") || (cta.textContent || "").trim().slice(0, 80) });
    if (target.closest("[data-vz-consent-open]")) {
      event.preventDefault();
      const panel = one(doc, "vz-consent");
      if (panel) {
        consentPanel(panel);
        panel.hidden = false;
        const first = one(panel, "button");
        if (first) first.focus();
      }
    }
    const dialog = target.closest("dialog[open]");
    if (dialog && target.closest("[data-vz-close]")) closePopup(dialog);
  });
  on(doc, "keydown", (event) => {
    if (event.key === "Enter" && event.target.tagName === "VZ-WHATSAPP") follow(event.target);
  });

  const pending = new Set();
  const flush = () => {
    for (const node of pending) if (node.isConnected) scan(node);
    pending.clear();
  };
  const schedule = (node) => {
    if (!pending.size) queueMicrotask(flush);
    pending.add(node);
  };
  new win.MutationObserver((records) => {
    for (const record of records) for (const node of record.addedNodes) if (node.nodeType === 1) schedule(node);
  }).observe(root, { childList: true, subtree: true });
  for (const tag of ["vz-form", "vz-whatsapp", "vz-consent", "vz-countdown", "vz-popup", "vz-webchat", "vz-booking"]) {
    if (!win.customElements.get(tag)) {
      win.customElements.define(tag, class extends win.HTMLElement {
        connectedCallback() {
          schedule(this);
        }
      });
    }
  }
  scan(root);
  beacon("view", { referrer: originOf(found.referrer), utm: found.utm });
}

if (typeof window !== "undefined" && window.document && window.customElements) startRuntime(window);

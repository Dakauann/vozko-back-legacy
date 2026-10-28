import { solveChallenge } from "./pow.js";

const MIN_FILL_MS = 3250;
const PROOF_REUSE_MS = 90000;
const DAY_MS = 86400000;
const TRAP_NAME = "vz_website";
const VZ = Symbol("vz");
const CONSENT_KEY = "vz-consent";
const BEACON_PATH = "/__vz/e";
const FORM_KEY = /^[A-Za-z0-9_-]{1,128}$/;
const LOCAL_ROUTE = /^\/(?![/\\])[^\\]*$/;
const UTM = ["source", "medium", "campaign", "term", "content"];
const FOCUSABLE = "a[href],button,input,select,textarea,[tabindex]";

const MESSAGE_KEYS = ["sending","success","failed","invalid","rateLimited","unavailable","consentTitle","consentText","acceptAll","rejectAll","customize","save","necessary","analytics","marketing","privacy"];

const MESSAGES = {
  pt: "Enviando...|Recebemos seus dados. Obrigado!|Não foi possível enviar. Tente de novo.|Confira os campos destacados.|Muitas tentativas. Aguarde um instante.|Formulário indisponível no momento.|Sua privacidade|Usamos cookies necessários e, se você permitir, de análise e de marketing.|Aceitar todos|Recusar opcionais|Personalizar|Salvar escolhas|Necessários|Análise|Marketing|Política de privacidade",
  en: "Sending...|We got your details. Thank you!|Could not send. Please try again.|Please check the highlighted fields.|Too many attempts. Please wait a moment.|This form is unavailable right now.|Your privacy|We use necessary cookies and, if you allow, analytics and marketing cookies.|Accept all|Reject optional|Customize|Save choices|Necessary|Analytics|Marketing|Privacy policy",
  es: "Enviando...|Recibimos tus datos. ¡Gracias!|No fue posible enviar. Inténtalo de nuevo.|Revisa los campos destacados.|Demasiados intentos. Espera un momento.|Formulario no disponible ahora.|Tu privacidad|Usamos cookies necesarias y, si lo permites, de análisis y de marketing.|Aceptar todas|Rechazar opcionales|Personalizar|Guardar elección|Necesarias|Análisis|Marketing|Política de privacidad",
  de: "Wird gesendet...|Wir haben Ihre Angaben erhalten. Danke!|Senden fehlgeschlagen. Bitte erneut versuchen.|Bitte prüfen Sie die markierten Felder.|Zu viele Versuche. Bitte kurz warten.|Formular derzeit nicht verfügbar.|Ihre Privatsphäre|Wir nutzen notwendige Cookies und, mit Ihrer Zustimmung, Analyse- und Marketing-Cookies.|Alle akzeptieren|Optionale ablehnen|Anpassen|Auswahl speichern|Notwendig|Analyse|Marketing|Datenschutzerklärung",
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

export async function fetchProof({ api, formKey, fetch: fetchImpl, solve, now }) {
  const reply = await postJson(fetchImpl, api + "/public/sites/forms/" + formKey + "/challenge", {});
  if (!reply.ok || !isRecord(reply.json)) throw new Error("challenge refused");
  const fetchedAt = now();
  return { challenge: reply.json, nonce: await solve(reply.json), fetchedAt };
}

const refusal = (reason, fields = []) => ({ ok: false, reason, fields });

export async function sendSubmission(run) {
  const { api, formKey, body, now, wait } = run;
  if (!isApiOrigin(api) || !FORM_KEY.test(formKey || "")) return refusal("unavailable");
  let proof = run.proof && now() - run.proof.fetchedAt < PROOF_REUSE_MS ? run.proof : null;
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      proof = proof || (await fetchProof(run));
      const pause = proof.fetchedAt + MIN_FILL_MS - now();
      if (pause > 0) await wait(pause);
      const reply = await postJson(run.fetch, api + "/public/sites/forms/" + formKey + "/submissions", { ...body, challengeToken: proof.challenge.token, nonce: proof.nonce });
      const code = isRecord(reply.json) && typeof reply.json.code === "string" ? reply.json.code : "";
      if (reply.ok) return isRecord(reply.json) ? { ok: true, success: successAction(reply.json.success) } : refusal("failed");
      if (code.startsWith("challenge_")) {
        if (attempt > 0) return refusal("failed");
        proof = null;
        continue;
      }
      if (reply.status === 429) return refusal("rate_limited");
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

  const beacon = (event, extra) => {
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
  const runInput = (form) => ({ api: (get(root, "data-vz-api") || "").replace(/\/$/, ""), formKey: keyOf(form), fetch: fetchImpl, solve, now, wait });

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
    const action = outcome.success;
    if (action.kind === "redirect") return go(action.route);
    if (action.kind === "whatsapp") {
      beacon("whatsapp", { target: action.url });
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

  const attachers = [
    ["vz-form form, form[data-vz-form]", attachForm],
    ["vz-popup > dialog, dialog[data-vz-popup]", attachPopup],
    ["vz-countdown, [data-vz-countdown]", attachCountdown],
    ["vz-consent", attachConsent],
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
  for (const tag of ["vz-form", "vz-whatsapp", "vz-consent", "vz-countdown", "vz-popup"]) {
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

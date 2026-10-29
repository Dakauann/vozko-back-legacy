import { expect, test } from "bun:test";

import { fetchSlots, groupSlots, messagesFor, sendSubmission } from "./vz-runtime.js";

const API = "https://api.vozkoia.com";
const NOW = Date.parse("2026-10-28T12:00:00Z");
const challenge = { id: "c1", token: "t1", bits: 8, expiresAt: "2026-10-28T12:02:00Z" };

function fakeApi(responses) {
  const calls = [];
  const fetch = async (url, init) => {
    calls.push({ url, method: init.method || "GET", credentials: init.credentials, body: init.body ? JSON.parse(init.body) : undefined });
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return new Response(JSON.stringify(next.body), { status: next.status });
  };
  return { fetch, calls };
}

function booking(overrides = {}) {
  let clock = NOW;
  return {
    api: API,
    formKey: "fk_agenda",
    path: "bookings",
    body: { idempotencyKey: "idem-1", website: "", route: "/", variant: "", fields: { "lead.name": "Ana", "field.horario": "2026-10-29T12:00:00Z" }, utm: {} },
    solve: async (c) => `nonce-for-${c.token}`,
    now: () => clock,
    wait: async (ms) => {
      clock += ms;
    },
    ...overrides,
  };
}

const slotsReply = {
  timeZone: "America/Sao_Paulo",
  durationMinutes: 30,
  fields: ["email"],
  consentText: "Autorizo o contato.",
  preview: false,
  slots: [
    { start: "2026-10-29T12:00:00Z", end: "2026-10-29T12:30:00Z" },
    { start: "2026-10-29T12:30:00Z", end: "2026-10-29T13:00:00Z" },
    { start: "2026-10-30T02:30:00Z", end: "2026-10-30T03:00:00Z" },
  ],
};

test("reads the free slots without credentials and keeps only well formed ones", async () => {
  const api = fakeApi([{ status: 200, body: { ...slotsReply, slots: [...slotsReply.slots, { start: "amanhã" }, "x"] } }]);
  const got = await fetchSlots({ api: API, formKey: "fk_agenda", fetch: api.fetch });
  expect(api.calls).toEqual([{ url: API + "/public/sites/forms/fk_agenda/slots", method: "GET", credentials: "omit", body: undefined }]);
  expect(got.slots.length).toBe(3);
  expect(got.timeZone).toBe("America/Sao_Paulo");
  expect(got.fields).toEqual(["email"]);
  expect(got.consentText).toBe("Autorizo o contato.");
});

test("treats a refused or garbled slot list as unavailable, never as free", async () => {
  for (const reply of [{ status: 503, body: { code: "booking_unavailable" } }, { status: 200, body: "ok" }, { status: 200, body: { slots: "x" } }, new TypeError("offline")]) {
    const api = fakeApi([reply]);
    expect(await fetchSlots({ api: API, formKey: "fk_agenda", fetch: api.fetch })).toBeNull();
  }
  expect(await fetchSlots({ api: "javascript:x", formKey: "fk_agenda", fetch: fakeApi([]).fetch })).toBeNull();
  expect(await fetchSlots({ api: API, formKey: "../x", fetch: fakeApi([]).fetch })).toBeNull();
});

test("groups slots by the visitor's own day and labels times in the visitor's zone", () => {
  const recife = groupSlots(slotsReply.slots, "America/Recife", "pt-BR");
  expect(recife.map((day) => day.key)).toEqual(["2026-10-29"]);
  expect(recife[0].times.map((time) => time.label)).toEqual(["09:00", "09:30", "23:30"]);
  const tokyo = groupSlots(slotsReply.slots, "Asia/Tokyo", "pt-BR");
  expect(tokyo.map((day) => day.key)).toEqual(["2026-10-29", "2026-10-30"]);
  expect(tokyo[1].times[0].start).toBe("2026-10-30T02:30:00Z");
  expect(tokyo[0].label.length).toBeGreaterThan(3);
});

test("books through the bookings path and returns the booked time", async () => {
  const api = fakeApi([{ status: 200, body: challenge }, { status: 200, body: { id: "s1", success: { kind: "message" }, booking: { start: "2026-10-29T12:00:00Z", end: "2026-10-29T12:30:00Z", timeZone: "America/Sao_Paulo", meetingLink: "https://meet.google.com/abc" } } }]);
  const outcome = await sendSubmission(booking({ fetch: api.fetch }));
  expect(api.calls[1].url).toBe(API + "/public/sites/forms/fk_agenda/bookings");
  expect(api.calls[1].body.fields["field.horario"]).toBe("2026-10-29T12:00:00Z");
  expect(outcome.ok).toBe(true);
  expect(outcome.booking).toEqual({ start: "2026-10-29T12:00:00Z", end: "2026-10-29T12:30:00Z", timeZone: "America/Sao_Paulo", meetingLink: "https://meet.google.com/abc" });
});

test("a taken slot and a calendar outage are their own refusals", async () => {
  const taken = fakeApi([{ status: 200, body: challenge }, { status: 409, body: { code: "booking_slot_taken" } }]);
  expect(await sendSubmission(booking({ fetch: taken.fetch }))).toEqual({ ok: false, reason: "taken", fields: [] });
  const down = fakeApi([{ status: 200, body: challenge }, { status: 503, body: { code: "booking_unavailable" } }]);
  expect(await sendSubmission(booking({ fetch: down.fetch }))).toEqual({ ok: false, reason: "unavailable", fields: [] });
});

test("drops a meeting link that is not an https address", async () => {
  const api = fakeApi([{ status: 200, body: challenge }, { status: 200, body: { id: "s1", booking: { start: "2026-10-29T12:00:00Z", end: "2026-10-29T12:30:00Z", timeZone: "America/Sao_Paulo", meetingLink: "javascript:alert(1)" } } }]);
  const outcome = await sendSubmission(booking({ fetch: api.fetch }));
  expect(outcome.booking.meetingLink).toBe("");
});

test("every language has the booking words", () => {
  for (const lang of ["pt-BR", "en", "es", "de"]) {
    const words = messagesFor(lang);
    for (const key of ["bookingLoading", "bookingEmpty", "bookingUnavailable", "bookingDays", "bookingTimes", "bookingZone", "bookingBusinessZone", "bookingName", "bookingPhone", "bookingEmail", "bookingMessage", "bookingSubmit", "bookingTaken", "bookingDone", "bookingMeet", "bookingPreview", "bookingChange", "bookingChosen"]) {
      expect(typeof words[key]).toBe("string");
      expect(words[key].length).toBeGreaterThan(1);
    }
  }
});

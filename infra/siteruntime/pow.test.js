import { expect, test } from "bun:test";

import { leadingZeroBits, solveChallenge } from "./pow.js";

function digestBits(token, nonce) {
  const hasher = new Bun.CryptoHasher("sha256");
  hasher.update(`${token}:${nonce}`);
  return leadingZeroBits(hasher.digest().buffer);
}

test("counts leading zero bits the way the Go verifier does", () => {
  expect(leadingZeroBits(new Uint8Array([0x80, 0]).buffer)).toBe(0);
  expect(leadingZeroBits(new Uint8Array([0x01, 0]).buffer)).toBe(7);
  expect(leadingZeroBits(new Uint8Array([0, 0, 0x0f]).buffer)).toBe(20);
  expect(leadingZeroBits(new Uint8Array(32).buffer)).toBe(256);
});

test("finds a decimal nonce whose sha256 of token:nonce has enough leading zero bits", async () => {
  const challenge = { id: "c1", token: "abc.widget.10.1790000000.sig", bits: 10, expiresAt: "2026-10-28T12:00:00Z" };
  const nonce = await solveChallenge(challenge);
  expect(nonce).toMatch(/^(0|[1-9][0-9]*)$/);
  expect(nonce.length).toBeLessThanOrEqual(20);
  expect(digestBits(challenge.token, nonce)).toBeGreaterThanOrEqual(10);
  for (let n = 0; n < Number(nonce); n++) expect(digestBits(challenge.token, String(n))).toBeLessThan(10);
});

test("refuses a challenge it cannot trust", async () => {
  for (const challenge of [null, {}, { token: "", bits: 8 }, { token: "t", bits: 0 }, { token: "t", bits: 33 }, { token: "t", bits: 1.5 }, { token: 7, bits: 8 }]) {
    await expect(solveChallenge(challenge)).rejects.toThrow();
  }
});

const BATCH = 512;
const MAX_BITS = 32;

export function leadingZeroBits(buffer) {
  const bytes = new Uint8Array(buffer);
  let total = 0;
  for (let i = 0; i < bytes.length; i++) {
    if (bytes[i] === 0) {
      total += 8;
      continue;
    }
    return total + Math.clz32(bytes[i]) - 24;
  }
  return total;
}

export async function solveChallenge(challenge) {
  const token = challenge && challenge.token;
  const bits = challenge && challenge.bits;
  if (typeof token !== "string" || token === "" || !Number.isInteger(bits) || bits < 1 || bits > MAX_BITS) {
    throw new Error("challenge refused");
  }
  const encoder = new TextEncoder();
  for (let start = 0; ; start += BATCH) {
    const work = [];
    for (let n = start; n < start + BATCH; n++) work.push(crypto.subtle.digest("SHA-256", encoder.encode(token + ":" + n)));
    const digests = await Promise.all(work);
    for (let i = 0; i < digests.length; i++) {
      if (leadingZeroBits(digests[i]) >= bits) return String(start + i);
    }
  }
}

export type EditLocation =
  | { ok: true; ranges: Array<[number, number]> }
  | { ok: false; code: "edit_not_found" | "edit_ambiguous"; detail: string };

const typographic: Record<string, string> = {
  "‘": "'", "’": "'", "‚": "'", "‛": "'", "′": "'",
  "“": '"', "”": '"', "„": '"', "‟": '"', "″": '"',
  "‐": "-", "‑": "-", "‒": "-", "–": "-", "—": "-", "―": "-", "−": "-",
  " ": " ", " ": " ", " ": " ", " ": " ", " ": " ", " ": " ", " ": " ",
  " ": " ", " ": " ", " ": " ", " ": " ", " ": " ", "　": " ",
};

const typographicPattern = new RegExp(`[${Object.keys(typographic).join("")}]`, "g");

export function normalizeTypography(text: string): string {
  return text.replace(typographicPattern, (c) => typographic[c]);
}

function occurrences(haystack: string, needle: string): number[] {
  const found: number[] = [];
  if (needle === "") return found;
  for (let at = haystack.indexOf(needle); at >= 0; at = haystack.indexOf(needle, at + needle.length)) found.push(at);
  return found;
}

interface Line {
  start: number;
  end: number;
  text: string;
}

function linesOf(text: string): Line[] {
  const lines: Line[] = [];
  let start = 0;
  for (let i = 0; i <= text.length; i++) {
    if (i === text.length || text[i] === "\n") {
      lines.push({ start, end: i, text: text.slice(start, i) });
      start = i + 1;
    }
  }
  return lines;
}

function trailingWhitespaceMatches(content: string, old: string): Array<[number, number]> {
  const contentLines = linesOf(content);
  const endsWithNewline = old.endsWith("\n");
  const oldLines = (endsWithNewline ? old.slice(0, -1) : old).split("\n").map((line) => line.replace(/\s+$/, ""));
  const ranges: Array<[number, number]> = [];
  for (let i = 0; i + oldLines.length <= contentLines.length; i++) {
    let same = true;
    for (let j = 0; j < oldLines.length && same; j++) same = contentLines[i + j].text.replace(/\s+$/, "") === oldLines[j];
    if (!same) continue;
    const last = contentLines[i + oldLines.length - 1];
    const end = endsWithNewline ? Math.min(last.end + 1, content.length) : last.end;
    ranges.push([contentLines[i].start, end]);
    i += oldLines.length - 1;
  }
  return ranges;
}

function lineNumberAt(content: string, offset: number): number {
  let line = 1;
  for (let i = 0; i < offset; i++) if (content.charCodeAt(i) === 10) line++;
  return line;
}

function bigrams(text: string): Map<string, number> {
  const out = new Map<string, number>();
  const clean = text.trim().toLowerCase();
  for (let i = 0; i < clean.length - 1; i++) {
    const pair = clean.slice(i, i + 2);
    out.set(pair, (out.get(pair) ?? 0) + 1);
  }
  return out;
}

function similarity(a: string, b: string): number {
  const left = bigrams(a);
  const right = bigrams(b);
  let total = 0;
  let shared = 0;
  for (const count of left.values()) total += count;
  for (const [pair, count] of right) {
    total += count;
    shared += Math.min(count, left.get(pair) ?? 0);
  }
  return total === 0 ? (a.trim() === b.trim() ? 1 : 0) : (2 * shared) / total;
}

const maxContextLines = 12;
const maxContextChars = 1200;

export function nearestContext(content: string, old: string): string {
  const contentLines = linesOf(content);
  const oldLines = old.split("\n").filter((line) => line.trim() !== "");
  if (oldLines.length === 0 || contentLines.length === 0) return "";
  const window = Math.max(1, Math.min(oldLines.length, maxContextLines));
  let best = 0;
  let bestScore = -1;
  for (let i = 0; i < contentLines.length; i++) {
    let score = 0;
    for (let j = 0; j < window && i + j < contentLines.length; j++) score += similarity(contentLines[i + j].text, oldLines[j] ?? "");
    if (score > bestScore) {
      bestScore = score;
      best = i;
    }
  }
  const shown = contentLines.slice(best, best + window).map((line, k) => `${best + k + 1}: ${line.text}`).join("\n");
  return `nearest match at line ${best + 1}\n${shown.length > maxContextChars ? shown.slice(0, maxContextChars) : shown}`;
}

export function locateEdit(content: string, old: string, replaceAll: boolean): EditLocation {
  const normalizedContent = normalizeTypography(content);
  const normalizedOld = normalizeTypography(old);
  const strategies: Array<() => Array<[number, number]>> = [
    () => occurrences(content, old).map((at) => [at, at + old.length]),
    () => occurrences(normalizedContent, normalizedOld).map((at) => [at, at + normalizedOld.length]),
    () => trailingWhitespaceMatches(normalizedContent, normalizedOld),
  ];
  for (const strategy of strategies) {
    const ranges = strategy();
    if (ranges.length === 0) continue;
    if (ranges.length > 1 && !replaceAll) {
      const lines = ranges.map(([start]) => lineNumberAt(content, start));
      return { ok: false, code: "edit_ambiguous", detail: `${ranges.length} matches at lines ${lines.join(", ")}` };
    }
    return { ok: true, ranges };
  }
  return { ok: false, code: "edit_not_found", detail: nearestContext(content, old) };
}

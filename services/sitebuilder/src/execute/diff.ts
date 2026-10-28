import { diffChars, diffLines } from "diff";
import type { Splice } from "./patcher";

const refineLimit = 20000;
const refineEditLength = 4000;

interface Change {
  value: string;
  added?: boolean;
  removed?: boolean;
}

function changesToSplices(changes: readonly Change[], offset: number): Splice[] {
  const splices: Splice[] = [];
  let position = offset;
  let pending: Splice | null = null;
  const flush = () => {
    if (pending) splices.push(pending);
    pending = null;
  };
  for (const change of changes) {
    if (change.removed) {
      if (!pending) pending = { start: position, end: position, length: 0 };
      pending.end += change.value.length;
      position += change.value.length;
      continue;
    }
    if (change.added) {
      if (!pending) pending = { start: position, end: position, length: 0 };
      pending.length += change.value.length;
      continue;
    }
    flush();
    position += change.value.length;
  }
  flush();
  return splices;
}

function refine(before: string, after: string, offset: number): Splice[] {
  if (before.length > refineLimit || after.length > refineLimit) return [{ start: offset, end: offset + before.length, length: after.length }];
  const chars = diffChars(before, after, { maxEditLength: refineEditLength }) as Change[] | undefined;
  if (!chars) return [{ start: offset, end: offset + before.length, length: after.length }];
  return changesToSplices(chars, offset);
}

export function diffSplices(before: string, after: string): Splice[] {
  if (before === after) return [];
  const lines = diffLines(before, after) as Change[];
  const splices: Splice[] = [];
  let position = 0;
  for (let i = 0; i < lines.length; i++) {
    const change = lines[i];
    if (!change.added && !change.removed) {
      position += change.value.length;
      continue;
    }
    let removed = "";
    let added = "";
    while (i < lines.length && (lines[i].added || lines[i].removed)) {
      if (lines[i].removed) removed += lines[i].value;
      else added += lines[i].value;
      i++;
    }
    i--;
    splices.push(...refine(removed, added, position));
    position += removed.length;
  }
  return splices;
}

import MagicString from "magic-string";

export interface Splice {
  start: number;
  end: number;
  length: number;
}

interface Edit {
  start: number;
  end: number;
  text: string;
  order: number;
}

export class OverlapError extends Error {}

export class Patcher {
  private readonly source: string;
  private readonly magic: MagicString;
  private readonly edits: Edit[] = [];

  constructor(source: string) {
    this.source = source;
    this.magic = new MagicString(source);
  }

  get original(): string {
    return this.source;
  }

  replace(start: number, end: number, text: string): this {
    if (start === end) return this.insertBefore(start, text);
    this.claim(start, end, text);
    if (text === "") this.magic.remove(start, end);
    else this.magic.update(start, end, text);
    return this;
  }

  remove(start: number, end: number): this {
    return this.replace(start, end, "");
  }

  insertBefore(index: number, text: string): this {
    if (text === "") return this;
    this.claim(index, index, text);
    this.magic.prependRight(index, text);
    return this;
  }

  insertAfter(index: number, text: string): this {
    if (text === "") return this;
    this.claim(index, index, text);
    this.magic.appendLeft(index, text);
    return this;
  }

  get changed(): boolean {
    return this.edits.length > 0;
  }

  result(): { content: string; splices: Splice[] } {
    const content = this.magic.toString();
    const ordered = [...this.edits].sort((a, b) => a.start - b.start || (a.end - a.start) - (b.end - b.start) || a.order - b.order);
    const splices: Splice[] = [];
    for (const edit of ordered) {
      const last = splices[splices.length - 1];
      if (last && last.start === edit.start && last.end === edit.start && edit.end === edit.start) {
        last.length += edit.text.length;
        continue;
      }
      splices.push({ start: edit.start, end: edit.end, length: edit.text.length });
    }
    return { content, splices };
  }

  private claim(start: number, end: number, text: string) {
    if (start < 0 || end > this.source.length || start > end) throw new OverlapError("range outside the source");
    for (const edit of this.edits) {
      const bothRanges = edit.end > edit.start && end > start;
      if (bothRanges && start < edit.end && edit.start < end) throw new OverlapError("overlapping edits");
      if (end === start && edit.end > edit.start && start > edit.start && start < edit.end) throw new OverlapError("insert inside an edit");
      if (edit.end === edit.start && end > start && edit.start > start && edit.start < end) throw new OverlapError("edit over an insert");
    }
    this.edits.push({ start, end, text, order: this.edits.length });
  }
}

export function mapOffset(offset: number, splices: readonly Splice[]): number | null {
  let shift = 0;
  for (const splice of splices) {
    if (splice.start === splice.end) {
      if (splice.start <= offset) shift += splice.length;
      continue;
    }
    if (offset >= splice.end) {
      shift += splice.length - (splice.end - splice.start);
      continue;
    }
    if (offset >= splice.start) return null;
  }
  return offset + shift;
}

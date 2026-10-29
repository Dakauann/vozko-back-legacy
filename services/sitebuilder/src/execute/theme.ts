import { Patcher, type Splice } from "./patcher";

interface Block {
  start: number;
  open: number;
  close: number;
}

function skipComment(css: string, i: number): number {
  const end = css.indexOf("*/", i + 2);
  return end < 0 ? css.length : end + 2;
}

function skipString(css: string, i: number): number {
  const quote = css[i];
  for (let j = i + 1; j < css.length; j++) {
    if (css[j] === "\\") j++;
    else if (css[j] === quote || css[j] === "\n") return j + 1;
  }
  return css.length;
}

function matchingBrace(css: string, open: number): number {
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css.startsWith("/*", i)) {
      i = skipComment(css, i) - 1;
      continue;
    }
    if (css[i] === '"' || css[i] === "'") {
      i = skipString(css, i) - 1;
      continue;
    }
    if (css[i] === "{") depth++;
    else if (css[i] === "}" && --depth === 0) return i;
  }
  return -1;
}

function themeBlocks(css: string): Block[] {
  const blocks: Block[] = [];
  for (let i = 0; i < css.length; i++) {
    if (css.startsWith("/*", i)) {
      i = skipComment(css, i) - 1;
      continue;
    }
    if (css[i] === '"' || css[i] === "'") {
      i = skipString(css, i) - 1;
      continue;
    }
    if (!css.startsWith("@theme", i) || /[\w-]/.test(css[i + 6] ?? "")) continue;
    const open = css.indexOf("{", i);
    const semicolon = css.indexOf(";", i);
    if (open < 0 || (semicolon >= 0 && semicolon < open)) continue;
    const close = matchingBrace(css, open);
    if (close < 0) continue;
    blocks.push({ start: i, open, close });
    i = close;
  }
  return blocks;
}

interface Declaration {
  valueStart: number;
  valueEnd: number;
}

function findDeclaration(css: string, name: string, from: number, to: number, topLevelOnly: boolean): Declaration | null {
  let depth = 0;
  for (let i = from; i < to; i++) {
    if (css.startsWith("/*", i)) {
      i = skipComment(css, i) - 1;
      continue;
    }
    if (css[i] === '"' || css[i] === "'") {
      i = skipString(css, i) - 1;
      continue;
    }
    if (css[i] === "{") depth++;
    else if (css[i] === "}") depth--;
    if (topLevelOnly && depth !== 0) continue;
    if (!css.startsWith(name, i) || /[\w-]/.test(css[i - 1] ?? "") ) continue;
    let j = i + name.length;
    while (j < to && /\s/.test(css[j])) j++;
    if (css[j] !== ":") continue;
    j++;
    let start = j;
    while (start < to && /\s/.test(css[start])) start++;
    let end = start;
    let parens = 0;
    while (end < to) {
      const c = css[end];
      if (c === '"' || c === "'") {
        end = skipString(css, end);
        continue;
      }
      if (c === "(") parens++;
      else if (c === ")") parens--;
      else if (parens === 0 && (c === ";" || c === "}")) break;
      end++;
    }
    let trimmed = end;
    while (trimmed > start && /\s/.test(css[trimmed - 1])) trimmed--;
    return { valueStart: start, valueEnd: trimmed };
  }
  return null;
}

export interface CssEdit {
  content: string;
  splices: Splice[];
}

function lineStartOf(css: string, index: number): number {
  let i = index;
  while (i > 0 && css[i - 1] !== "\n") i--;
  return i;
}

function declarationIndent(css: string, block: Block): string {
  const inner = css.slice(block.open + 1, block.close);
  const match = /\n([ \t]+)--/.exec(inner);
  return match ? match[1] : "  ";
}

export function themeSource(css: string): string {
  return themeBlocks(css).map((block) => css.slice(block.start, block.close + 1)).join("\n");
}

export function setThemeToken(css: string, name: string, value: string): CssEdit {
  const patcher = new Patcher(css);
  const blocks = themeBlocks(css);
  for (const block of blocks) {
    const found = findDeclaration(css, name, block.open + 1, block.close, true);
    if (found) {
      patcher.replace(found.valueStart, found.valueEnd, value);
      return patcher.result();
    }
  }
  if (blocks.length > 0) {
    const block = blocks[0];
    const indent = declarationIndent(css, block);
    const closeLine = lineStartOf(css, block.close);
    if (/^[ \t]*$/.test(css.slice(closeLine, block.close)) && closeLine > block.open) {
      patcher.insertBefore(closeLine, `${indent}${name}: ${value};\n`);
    } else {
      const before = css.slice(block.open + 1, block.close).trim();
      const separator = before === "" || before.endsWith(";") ? " " : "; ";
      patcher.insertBefore(block.close, `${separator}${name}: ${value}; `);
    }
    return patcher.result();
  }
  const importMatch = /@import\s+["']tailwindcss["'][^;]*;[ \t]*\n?/.exec(css);
  const block = `@theme {\n  ${name}: ${value};\n}\n`;
  if (importMatch) {
    const at = importMatch.index + importMatch[0].length;
    patcher.insertBefore(at, (importMatch[0].endsWith("\n") ? "" : "\n") + block);
  } else {
    patcher.insertBefore(0, block);
  }
  return patcher.result();
}

export function setCssVariable(css: string, name: string, value: string): CssEdit | null {
  for (const block of themeBlocks(css)) {
    const found = findDeclaration(css, name, block.open + 1, block.close, true);
    if (found) return new Patcher(css).replace(found.valueStart, found.valueEnd, value).result();
  }
  const anywhere = findDeclaration(css, name, 0, css.length, false);
  if (!anywhere) return null;
  return new Patcher(css).replace(anywhere.valueStart, anywhere.valueEnd, value).result();
}

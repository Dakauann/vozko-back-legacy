export interface Run {
  text: string;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  link?: { href: string };
}

export type MarkupTarget = "html" | "jsx";

const jsxSpecial = /&(?=[#a-zA-Z0-9]+;)|[{}<>]/g;

export function escapeHtmlText(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

export function escapeHtmlAttribute(value: string): string {
  return value.replace(/&/g, "&amp;").replace(/"/g, "&quot;");
}

export function escapeJsxText(text: string): string {
  return text.replace(jsxSpecial, (c) => `{"${c}"}`);
}

export function jsxAttributeValue(value: string): string {
  if (/["&\\]/.test(value) || /[\n\r]/.test(value)) return `{${JSON.stringify(value)}}`;
  return `"${value}"`;
}

export function htmlAttributeValue(value: string): string {
  return `"${escapeHtmlAttribute(value)}"`;
}

function lineBreak(target: MarkupTarget): string {
  return target === "jsx" ? "<br />" : "<br>";
}

function renderText(text: string, target: MarkupTarget): string {
  const escape = target === "jsx" ? escapeJsxText : escapeHtmlText;
  return text.split("\n").map(escape).join(lineBreak(target));
}

function renderMarks(run: Run, target: MarkupTarget): string {
  let out = renderText(run.text, target);
  if (run.underline) out = `<u>${out}</u>`;
  if (run.italic) out = `<em>${out}</em>`;
  if (run.bold) out = `<strong>${out}</strong>`;
  return out;
}

function hrefAttribute(href: string, target: MarkupTarget): string {
  return target === "jsx" ? jsxAttributeValue(href) : htmlAttributeValue(href);
}

export function renderRuns(runs: readonly Run[], target: MarkupTarget): string {
  let out = "";
  for (let i = 0; i < runs.length; i++) {
    const link = runs[i].link;
    if (!link) {
      out += renderMarks(runs[i], target);
      continue;
    }
    let inner = "";
    while (i < runs.length && runs[i].link?.href === link.href) {
      inner += renderMarks(runs[i], target);
      i++;
    }
    i--;
    out += `<a href=${hrefAttribute(link.href, target)}>${inner}</a>`;
  }
  return out;
}

export function plainText(runs: readonly Run[]): string | null {
  let out = "";
  for (const run of runs) {
    if (run.bold || run.italic || run.underline || run.link) return null;
    out += run.text;
  }
  return out;
}

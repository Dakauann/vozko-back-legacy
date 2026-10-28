import type { Route } from "../project";
import { routeURL, siteOrigin, type ShellInput } from "./shell";

export interface SiteFile {
  path: string;
  body: string;
}

function escapeXml(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&apos;");
}

function oneLine(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

export function siteFiles(siteName: string, routes: Route[], shell: ShellInput): SiteFile[] {
  const indexed = routes.filter((route) => !shell.pages[route.path]?.noindex);
  const sitemap = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">',
    ...indexed.map((route) => `  <url><loc>${escapeXml(routeURL(shell, route.path))}</loc></url>`),
    "</urlset>",
    "",
  ].join("\n");
  const robots = indexed.length === 0
    ? "User-agent: *\nDisallow: /\n"
    : `User-agent: *\nAllow: /\nSitemap: ${siteOrigin(shell)}/sitemap.xml\n`;
  const home = shell.pages["/"] ?? {};
  const llms = [`# ${oneLine(home.title || siteName)}`, ""];
  if (home.description) llms.push(`> ${oneLine(home.description)}`, "");
  llms.push("## Páginas", "");
  for (const route of indexed) {
    const meta = shell.pages[route.path] ?? {};
    const title = oneLine(meta.title || (route.path === "/" ? siteName : route.path));
    llms.push(`- [${title}](${routeURL(shell, route.path)})${meta.description ? `: ${oneLine(meta.description)}` : ""}`);
  }
  llms.push("");
  return [
    { path: "sitemap.xml", body: sitemap },
    { path: "robots.txt", body: robots },
    { path: "llms.txt", body: llms.join("\n") },
  ];
}

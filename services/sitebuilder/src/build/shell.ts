import { parse, serialize } from "parse5";
import { escapeHtmlAttribute, escapeHtmlText } from "../execute/inline";

export type BuildMode = "preview" | "publish";

export interface PageMeta {
  title?: string;
  description?: string;
  canonical?: string;
  og_image?: string;
  noindex?: boolean;
}

export interface ShellInput {
  api: string;
  forms: Record<string, string>;
  variant?: string;
  pages: Record<string, PageMeta>;
  integrations: Record<string, string>;
  loaders: Record<string, string>;
  consent_text?: string;
  primary_origin?: string;
  favicon?: string;
  structured_data?: unknown[];
}

export const originPlaceholder = "__VZ_ORIGIN__";

export function siteOrigin(shell: ShellInput): string {
  return shell.primary_origin || originPlaceholder;
}

export function routeURL(shell: ShellInput, route: string): string {
  return siteOrigin(shell) + route;
}

function jsonForScript(value: unknown): string {
  const escapes: Record<string, string> = { "<": "u003c", ">": "u003e", "&": "u0026" };
  return JSON.stringify(value).replace(/[<>&]/g, (c) => String.fromCharCode(92) + escapes[c]);
}

export interface IntegrationScript {
  kind: string;
  category: "analytics" | "marketing";
  src: string;
}

export interface ShellParts {
  mode: BuildMode;
  lang: string;
  siteName: string;
  route: string;
  shell: ShellInput;
  revs: Record<string, number>;
  styles: string[];
  preloads: string[];
  consentDefault?: string;
  runtime: string;
  bridge?: string;
  integrations: IntegrationScript[];
  body: string;
  appScripts: string[];
  classicScripts?: string[];
}

export const prerenderPlaceholder = "<!--vz-prerender-->";

function attr(name: string, value: string): string {
  return ` ${name}="${escapeHtmlAttribute(value)}"`;
}

export function renderShell(parts: ShellParts): string {
  const meta = parts.shell.pages[parts.route] ?? {};
  const title = meta.title?.trim() || parts.siteName;
  const head: string[] = [
    '<meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width, initial-scale=1">',
    `<title>${escapeHtmlText(title)}</title>`,
  ];
  if (meta.description) head.push(`<meta name="description"${attr("content", meta.description)}>`);
  if (parts.mode === "preview" || meta.noindex) head.push('<meta name="robots" content="noindex">');
  const canonical = meta.canonical || (parts.mode === "publish" ? routeURL(parts.shell, parts.route) : "");
  if (canonical) head.push(`<link rel="canonical"${attr("href", canonical)}>`);
  if (parts.shell.favicon) head.push(`<link rel="icon"${attr("href", parts.shell.favicon)}>`);
  head.push(`<meta property="og:type" content="website">`);
  head.push(`<meta property="og:title"${attr("content", title)}>`);
  if (meta.description) head.push(`<meta property="og:description"${attr("content", meta.description)}>`);
  if (canonical) head.push(`<meta property="og:url"${attr("content", canonical)}>`);
  if (meta.og_image) {
    head.push(`<meta property="og:image"${attr("content", meta.og_image)}>`);
    head.push('<meta name="twitter:card" content="summary_large_image">');
  }
  if (parts.mode === "preview") head.push(`<meta name="vz-revs"${attr("content", JSON.stringify(parts.revs))}>`);
  if (parts.consentDefault) head.push(`<script${attr("src", parts.consentDefault)}></script>`);
  for (const data of parts.shell.structured_data ?? []) head.push(`<script type="application/ld+json">${jsonForScript(data)}</script>`);
  for (const href of parts.styles) head.push(`<link rel="stylesheet"${attr("href", href)}>`);
  for (const href of parts.preloads) head.push(`<link rel="modulepreload"${attr("href", href)}>`);
  head.push(`<script type="module"${attr("src", parts.runtime)}></script>`);
  for (const script of parts.integrations) {
    head.push(`<script type="text/plain"${attr("data-vz-consent", script.category)}${attr("data-vz-integration", script.kind)}${attr("src", script.src)}></script>`);
  }
  const body: string[] = [parts.body];
  for (const src of parts.appScripts) body.push(`<script type="module" data-vz-app${attr("src", src)}></script>`);
  if (parts.bridge) body.push(`<script type="module"${attr("src", parts.bridge)}></script>`);
  let html = `<html${attr("lang", parts.lang)}${attr("data-vz-api", parts.shell.api)}${attr("data-vz-forms", JSON.stringify(parts.shell.forms))}${attr("data-vz-route", parts.route)}`;
  if (parts.shell.variant) html += attr("data-vz-variant", parts.shell.variant);
  return normalizeDocument(`<!doctype html>\n${html}><head>${head.join("")}</head><body>${body.join("")}</body></html>`);
}

export function normalizeDocument(html: string): string {
  return serialize(parse(html));
}

const integrationIds: Record<string, RegExp> = {
  meta_pixel: /^\d{10,20}$/,
  ga4: /^G-[A-Z0-9]{4,16}$/,
  gtm: /^GTM-[A-Z0-9]{4,12}$/,
  google_ads: /^AW-\d{6,14}(\/[A-Za-z0-9_-]{1,64})?$/,
  tiktok_pixel: /^[A-Z0-9]{10,32}$/,
  clarity: /^[a-z0-9]{6,16}$/,
};

export class IntegrationRefused extends Error {}

function gtagLoader(loader: string, id: string, configId: string): string {
  return `(function(){var s=document.createElement("script");s.async=true;s.src=${JSON.stringify(loader + "?id=")}+encodeURIComponent(${JSON.stringify(id)});document.head.appendChild(s);window.dataLayer=window.dataLayer||[];function gtag(){window.dataLayer.push(arguments);}gtag("js",new Date());gtag("config",${JSON.stringify(configId)});})();\n`;
}

export function integrationCode(kind: string, id: string, loader: string | undefined): { category: "analytics" | "marketing"; code: string } {
  const pattern = integrationIds[kind];
  if (!pattern || !pattern.test(id)) throw new IntegrationRefused(`integration ${kind} has an invalid id`);
  if (!loader) throw new IntegrationRefused(`integration ${kind} has no loader`);
  const literal = JSON.stringify(id);
  const source = JSON.stringify(loader);
  switch (kind) {
    case "meta_pixel":
      return {
        category: "marketing",
        code: `!function(f,b,e,v,n,t,s){if(f.fbq)return;n=f.fbq=function(){n.callMethod?n.callMethod.apply(n,arguments):n.queue.push(arguments)};if(!f._fbq)f._fbq=n;n.push=n;n.loaded=!0;n.version="2.0";n.queue=[];t=b.createElement(e);t.async=!0;t.src=v;s=b.getElementsByTagName(e)[0];s.parentNode.insertBefore(t,s)}(window,document,"script",${source});fbq("init",${literal});fbq("track","PageView");\n`,
      };
    case "ga4":
      return { category: "analytics", code: gtagLoader(loader, id, id) };
    case "google_ads": {
      const account = id.split("/")[0];
      return { category: "marketing", code: gtagLoader(loader, account, account) };
    }
    case "gtm":
      return {
        category: "marketing",
        code: `(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({"gtm.start":new Date().getTime(),event:"gtm.js"});var f=d.getElementsByTagName(s)[0],j=d.createElement(s);j.async=true;j.src=${JSON.stringify(loader + "?id=")}+encodeURIComponent(i);f.parentNode.insertBefore(j,f);})(window,document,"script","dataLayer",${literal});\n`,
      };
    case "tiktok_pixel":
      return {
        category: "marketing",
        code: `!function(w,d,t){w.TiktokAnalyticsObject=t;var ttq=w[t]=w[t]||[];ttq.methods=["page","track","identify","instances","debug","on","off","once","ready","alias","group","enableCookie","disableCookie"];ttq.setAndDefer=function(t,e){t[e]=function(){t.push([e].concat(Array.prototype.slice.call(arguments,0)))}};for(var i=0;i<ttq.methods.length;i++)ttq.setAndDefer(ttq,ttq.methods[i]);ttq.load=function(e){var n=${source};ttq._i=ttq._i||{};ttq._i[e]=[];ttq._t=ttq._t||{};ttq._t[e]=+new Date;var o=d.createElement("script");o.type="text/javascript";o.async=!0;o.src=n+"?sdkid="+encodeURIComponent(e)+"&lib="+t;var a=d.getElementsByTagName("script")[0];a.parentNode.insertBefore(o,a)};ttq.load(${literal});ttq.page()}(window,document,"ttq");\n`,
      };
    case "clarity":
      return {
        category: "analytics",
        code: `(function(c,l,a,r,i,t,y){c[a]=c[a]||function(){(c[a].q=c[a].q||[]).push(arguments)};t=l.createElement(r);t.async=1;t.src=${source}+encodeURIComponent(i);y=l.getElementsByTagName(r)[0];y.parentNode.insertBefore(t,y);})(window,document,"clarity","script",${literal});\n`,
      };
  }
  throw new IntegrationRefused(`integration ${kind} is unknown`);
}

export const consentDefaultCode =
  'window.dataLayer=window.dataLayer||[];function gtag(){window.dataLayer.push(arguments);}gtag("consent","default",{analytics_storage:"denied",ad_storage:"denied",ad_user_data:"denied",ad_personalization:"denied",wait_for_update:500});\n';

const reactNames: Record<string, string> = {
  class: "className",
  for: "htmlFor",
  tabindex: "tabIndex",
  readonly: "readOnly",
  maxlength: "maxLength",
  minlength: "minLength",
  autocomplete: "autoComplete",
  autofocus: "autoFocus",
  autoplay: "autoPlay",
  colspan: "colSpan",
  rowspan: "rowSpan",
  crossorigin: "crossOrigin",
  srcset: "srcSet",
  srcdoc: "srcDoc",
  enctype: "encType",
  novalidate: "noValidate",
  playsinline: "playsInline",
  frameborder: "frameBorder",
  allowfullscreen: "allowFullScreen",
  contenteditable: "contentEditable",
  spellcheck: "spellCheck",
  datetime: "dateTime",
  inputmode: "inputMode",
  enterkeyhint: "enterKeyHint",
  fetchpriority: "fetchPriority",
  referrerpolicy: "referrerPolicy",
  itemprop: "itemProp",
  itemscope: "itemScope",
  itemtype: "itemType",
  formaction: "formAction",
  formmethod: "formMethod",
  formnovalidate: "formNoValidate",
  formtarget: "formTarget",
  usemap: "useMap",
  accesskey: "accessKey",
  cellpadding: "cellPadding",
  cellspacing: "cellSpacing",
  charset: "charSet",
  "accept-charset": "acceptCharset",
  "http-equiv": "httpEquiv",
  "xlink:href": "xlinkHref",
  "xml:lang": "xmlLang",
  "xml:space": "xmlSpace",
};

export function reactAttributeName(name: string): string {
  const known = reactNames[name];
  if (known) return known;
  if (name.startsWith("data-") || name.startsWith("aria-") || !name.includes("-")) return name;
  return name.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
}

export function reactAttributeAliases(name: string): string[] {
  const react = reactAttributeName(name);
  return react === name ? [name] : [react, name];
}

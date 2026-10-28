(() => {
  if (globalThis.__vzProbe) return;
  const MAX_SCAN = 5000;

  let paint = null;
  const colors = new Map();
  const rgba = (value) => {
    if (colors.has(value)) return colors.get(value);
    if (!paint) {
      const canvas = document.createElement("canvas");
      canvas.width = 1;
      canvas.height = 1;
      paint = canvas.getContext("2d", { willReadFrequently: true });
    }
    paint.clearRect(0, 0, 1, 1);
    paint.fillStyle = "rgba(0, 0, 0, 0)";
    paint.fillStyle = value || "rgba(0, 0, 0, 0)";
    paint.fillRect(0, 0, 1, 1);
    const data = paint.getImageData(0, 0, 1, 1).data;
    const out = [data[0], data[1], data[2], data[3] / 255];
    colors.set(value, out);
    return out;
  };
  const round3 = (c) => [Math.round(c[0]), Math.round(c[1]), Math.round(c[2])];
  const frames = (n) =>
    new Promise((resolve) => {
      const step = (left) => (left <= 0 ? resolve() : requestAnimationFrame(() => step(left - 1)));
      step(n);
    });
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

  const pageHeight = () =>
    Math.round(Math.max(document.documentElement.scrollHeight, document.body ? document.body.scrollHeight : 0, innerHeight));

  const visit = async (top, settle) => {
    window.scrollTo({ top, left: 0, behavior: "instant" });
    await frames(2);
    if (settle > 0) await sleep(settle);
    return { y: Math.round(scrollY) };
  };

  const scrollOnce = async () => {
    window.scrollTo({ top: pageHeight(), left: 0, behavior: "instant" });
    await frames(2);
    await sleep(200);
    window.scrollTo({ top: 0, left: 0, behavior: "instant" });
    await frames(2);
    return true;
  };

  const settled = async (ms) => {
    try {
      await Promise.race([document.fonts.ready, sleep(3000)]);
    } catch (error) {}
    await frames(2);
    await sleep(ms);
    return true;
  };

  const ownText = (el) => {
    let length = 0;
    for (const child of el.childNodes) if (child.nodeType === 3) length += child.nodeValue.trim().length;
    return length;
  };

  const tokens = () => {
    const backgrounds = [];
    const textColors = [];
    const fonts = [];
    const spacing = [];
    const root = getComputedStyle(document.documentElement);
    const body = document.body ? getComputedStyle(document.body) : root;
    const canvas = rgba(body.backgroundColor)[3] > 0 ? rgba(body.backgroundColor) : rgba(root.backgroundColor);
    const pageArea = innerWidth * pageHeight();
    backgrounds.push({ rgb: round3(canvas[3] > 0 ? canvas : [255, 255, 255, 1]), w: pageArea });
    const all = document.body ? [...document.body.querySelectorAll("*")].slice(0, MAX_SCAN) : [];
    for (const el of all) {
      const r = el.getBoundingClientRect();
      if (r.width < 1 || r.height < 1) continue;
      const style = getComputedStyle(el);
      if (style.display === "none" || style.visibility === "hidden") continue;
      const bg = rgba(style.backgroundColor);
      if (bg[3] > 0.5) backgrounds.push({ rgb: round3(bg), w: Math.min(r.width, innerWidth) * Math.min(r.height, 3000) });
      const length = ownText(el);
      if (length > 0) {
        const px = parseFloat(style.fontSize) || 16;
        textColors.push({ rgb: round3(rgba(style.color)), w: length * px });
        fonts.push({ family: style.fontFamily.split(",")[0].trim(), w: length * px });
      }
      if (spacing.length < MAX_SCAN) {
        for (const value of [style.paddingTop, style.paddingRight, style.paddingBottom, style.paddingLeft, style.marginTop, style.marginBottom, style.rowGap, style.columnGap]) {
          const px = parseFloat(value);
          if (px > 0) spacing.push(px);
        }
      }
    }
    return { backgrounds, texts: textColors, fonts, spacing };
  };

  const describe = () => ({ url: location.href, title: document.title || "" });

  Object.defineProperty(globalThis, "__vzProbe", {
    value: Object.freeze({ visit, settled, scrollOnce, pageHeight, tokens, describe }),
    configurable: false,
    writable: false,
  });
})();

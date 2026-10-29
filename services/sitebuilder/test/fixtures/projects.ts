import { readFileSync } from "node:fs";
import type { Project } from "../../src/project";
import type { ShellInput } from "../../src/build/shell";

const runtimeDir = new URL("../../../../infra/siteruntime/", import.meta.url);

export const runtime: Record<string, string> = {
  "vz-runtime.js": readFileSync(new URL("vz-runtime.js", runtimeDir), "utf8"),
  "pow.js": readFileSync(new URL("pow.js", runtimeDir), "utf8"),
  "vz-editor-bridge.js": readFileSync(new URL("vz-editor-bridge.js", runtimeDir), "utf8"),
  "vz-screenshot.js": readFileSync(new URL("vz-screenshot.js", runtimeDir), "utf8"),
  "vz-tailwind-browser.js": readFileSync(new URL("vz-tailwind-browser.js", runtimeDir), "utf8"),
};

function project(stack: Project["stack"], files: Record<string, string>): Project {
  return { stack, files: Object.fromEntries(Object.entries(files).map(([path, content]) => [path, { content, rev: 1 }])) };
}

export const reactProject = project("react", {
  "site.json": '{"name":"Clínica Sorriso","lang":"pt-BR","routes":[{"path":"/","page":"src/pages/Home.jsx"},{"path":"/obrigado","page":"src/pages/Obrigado.jsx"}]}',
  "src/index.css": '@import "tailwindcss";\n@theme {\n  --color-brand: #0f766e;\n}\n',
  "src/main.jsx":
    'import { StrictMode } from "react";\nimport { createRoot, hydrateRoot } from "react-dom/client";\nimport "./index.css";\nimport App from "./App.jsx";\n\nconst root = document.getElementById("root");\nconst app = (\n  <StrictMode>\n    <App />\n  </StrictMode>\n);\n\nif (root.hasChildNodes()) {\n  hydrateRoot(root, app);\n} else {\n  createRoot(root).render(app);\n}\n',
  "src/App.jsx":
    'import Home from "./pages/Home.jsx";\nimport Obrigado from "./pages/Obrigado.jsx";\n\nconst routes = { "/": Home, "/obrigado": Obrigado };\n\nexport default function App() {\n  const path = typeof window === "undefined" ? "/" : window.location.pathname.replace(/\\/+$/, "") || "/";\n  const Page = routes[path] ?? Home;\n  return (\n    <div className="min-h-screen bg-white">\n      <Page />\n    </div>\n  );\n}\n',
  "src/lib/utils.js": 'import { clsx } from "clsx";\nimport { twMerge } from "tailwind-merge";\n\nexport function cn(...inputs) {\n  return twMerge(clsx(inputs));\n}\n',
  "src/components/Hero.jsx": 'export function Hero({ title }) {\n  return (\n    <section className="bg-brand py-20 text-white">\n      <h1 className="text-4xl font-bold">{title}</h1>\n    </section>\n  );\n}\n',
  "src/pages/Home.jsx":
    'import { useState } from "react";\nimport { motion } from "framer-motion";\nimport { Check } from "lucide-react";\nimport { Hero } from "../components/Hero";\nimport { cn } from "@/lib/utils";\n\nconst items = ["Implante", "Clareamento"];\n\nexport default function Home() {\n  const [open, setOpen] = useState(false);\n  return (\n    <main>\n      <Hero title="Bem-vindo" />\n      <ul className="grid gap-4">\n        {items.map((item) => (\n          <li key={item} className="flex items-center gap-2"><Check className="size-4" />{item}</li>\n        ))}\n      </ul>\n      <motion.h2 initial={{ opacity: 0 }} animate={{ opacity: 1 }} className={cn("text-2xl", open && "underline")}>Serviços</motion.h2>\n      <button type="button" className={open ? "font-bold" : "font-normal"} onClick={() => setOpen(!open)}>Mais</button>\n      <vz-form id="contato"><input name="lead.name" /></vz-form>\n    </main>\n  );\n}\n',
  "src/pages/Obrigado.jsx": 'export default function Obrigado() {\n  return <h1 className="text-3xl">Obrigado!</h1>;\n}\n',
});

export const htmlProject = project("html", {
  "site.json": '{"name":"Clínica Sorriso","lang":"pt-BR","routes":[{"path":"/","page":"pages/index.html"},{"path":"/obrigado","page":"pages/obrigado.html"}]}',
  "theme.css": '@import "tailwindcss";\n@theme {\n  --color-brand: oklch(0.6 0.2 250);\n}\n',
  "partials/header.html": '<header class="p-4">\n  <a href="/">Clínica</a>\n</header>\n',
  "pages/index.html":
    '<vz-include src="partials/header.html"></vz-include>\n<section class="bg-brand py-20">\n  <h1 class="text-4xl">Oi</h1>\n  <img src="https://cdn.vozko.test/a.webp" alt="Foto">\n</section>\n<script type="module" src="/scripts/motion.js"></script>\n',
  "pages/obrigado.html": '<h1 class="text-3xl">Obrigado</h1>\n',
  "scripts/motion.js": 'import { gsap } from "gsap";\n\ngsap.from("h1", { y: 20 });\n',
});

export const shell: ShellInput = {
  api: "https://api.vozko.test",
  forms: { contato: "fk_contato" },
  pages: {
    "/": { title: "Sorriso perfeito", description: "Implantes e clareamento", canonical: "https://sorriso.vozko.site/", og_image: "https://cdn.vozko.test/og.webp" },
    "/obrigado": { noindex: true },
  },
  integrations: { meta_pixel: "1234567890", ga4: "G-ABC1234" },
  loaders: { meta_pixel: "https://connect.facebook.net/en_US/fbevents.js", ga4: "https://www.googletagmanager.com/gtag/js" },
  consent_text: "Usamos cookies",
  primary_origin: "https://sorriso.vozko.site",
  favicon: "https://cdn.vozko.test/favicon.png",
  structured_data: [{ "@context": "https://schema.org", "@type": "Dentist", name: "Clínica </script> Sorriso" }],
};

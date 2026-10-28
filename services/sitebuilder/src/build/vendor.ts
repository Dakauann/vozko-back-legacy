import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export interface VendorModule {
  file: string;
  preload: string[];
}

export interface VendorManifest {
  version: string;
  modules: Record<string, VendorModule>;
  styles: Record<string, string>;
  inline: string[];
  html: string[];
}

export interface Vendor {
  manifest: VendorManifest;
  read(file: string): Uint8Array;
  url(file: string): string;
}

export const vendorDir = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "vendor");
export const vendorAssetPrefix = "assets/vendor/";

let loaded: Vendor | null = null;

export function loadVendor(dir = vendorDir): Vendor {
  if (loaded && dir === vendorDir) return loaded;
  const manifestPath = join(dir, "manifest.json");
  if (!existsSync(manifestPath)) throw new Error("the vendor folder is missing; run bun run vendor");
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as VendorManifest;
  const cache = new Map<string, Uint8Array>();
  const vendor: Vendor = {
    manifest,
    read(file: string) {
      if (!/^[A-Za-z0-9_.-]+$/.test(file)) throw new Error(`vendor file ${file} is not allowed`);
      let body = cache.get(file);
      if (!body) {
        body = new Uint8Array(readFileSync(join(dir, file)));
        cache.set(file, body);
      }
      return body;
    },
    url(file: string) {
      return `/${vendorAssetPrefix}${file}`;
    },
  };
  if (dir === vendorDir) loaded = vendor;
  return vendor;
}

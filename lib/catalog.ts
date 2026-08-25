import type { CatalogApp } from "./types";

export const DEFAULT_SOFTWARE: CatalogApp[] = [
  { id: "adobe-ae", name: "Adobe After Effects", match: "Adobe After Effects" },
  { id: "adobe-ps", name: "Adobe Photoshop", match: "Adobe Photoshop" },
  { id: "adobe-cc", name: "Adobe Creative Cloud", match: "Adobe Creative Cloud" },
  { id: "maxon", name: "Maxon", match: "Maxon" },
  { id: "c4d-2026", name: "Cinema 4D 2026", match: "Cinema 4D 2026" },
];

export function softwareNeedle(app: CatalogApp) {
  return (app.wingetId || app.match || app.name).trim();
}

export function slugSoftwareId(name: string) {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
  return slug || `app_${Date.now().toString(36)}`;
}

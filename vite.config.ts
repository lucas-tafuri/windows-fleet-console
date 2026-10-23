import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  root: "web",
  publicDir: "../public",
  plugins: [react(), {
    name: "keep-go-embed-directory",
    generateBundle() {
      this.emitFile({ type: "asset", fileName: ".gitkeep", source: "" });
    },
  }],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, ".") } },
  build: { outDir: "../agent/ui/dist", emptyOutDir: true },
  server: { host: "127.0.0.1", proxy: { "/api": { target: "http://127.0.0.1:43123", changeOrigin: false } } },
});

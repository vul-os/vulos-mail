import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Build the SPA into ./dist (served by the Go server as VULOS_WEBMAIL_DIR).
// Relative base so it works under any mount point.
export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  test: {
    environment: "jsdom",
    globals: true,
    // The mail UI (and its tests) now live in @vulos/mail-ui; the webmail shell
    // is a thin mount, so there are no local tests to run.
    passWithNoTests: true,
  },
});

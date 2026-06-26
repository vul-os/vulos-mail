import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Build the SPA into ./dist (served by the Go server as VULOS_WEBMAIL_DIR).
// Relative base so it works under any mount point.
//
// Dev proxy: the webmail talks to the vulos-mail HTTP server, which serves the
// session/login endpoints (/api/webmail/*), JMAP/DAV, and — crucially — the
// reverse-proxied lilmail mail engine at /v1 (folders, messages, search, send).
// Point all of those at the running vulos-mail server (default :2080, override
// with VULOS_DEV_BACKEND) so dev mirrors production end-to-end, including the
// /v1 → lilmail broker hop. (Run vulos-mail with LILMAIL_ENGINE_URL set so its
// /v1 actually reaches a lilmail engine.)
const backend = process.env.VULOS_DEV_BACKEND || "http://127.0.0.1:2080";

export default defineConfig({
  base: "./",
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/v1": { target: backend, changeOrigin: true },
      "/api": { target: backend, changeOrigin: true },
      "/jmap": { target: backend, changeOrigin: true },
      "/dav": { target: backend, changeOrigin: true },
    },
  },
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

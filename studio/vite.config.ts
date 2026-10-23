import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The Studio that the dev server proxies to: `restitch dev` serves it on
// 127.0.0.1:3080.
const apiTarget = process.env.VITE_API_TARGET ?? "http://127.0.0.1:3080";

export default defineConfig({
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true, routeFileIgnorePattern: "\\.test\\." }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "./src") },
  },
  build: {
    // internal/studio embeds the build, so it lands next to the Go package.
    outDir: "../internal/studio/dist",
    emptyOutDir: true,
    manifest: true,
    assetsDir: "assets",
    assetsInlineLimit: 0,
  },
  server: {
    proxy: {
      "/api": apiTarget,
    },
  },
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["./src/test-setup.ts"],
  },
});

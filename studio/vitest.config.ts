import { defineConfig } from "vitest/config"
import path from "path"

// Node 25 enables the experimental Web Storage API by default, installing a
// built-in `localStorage` global. Inside vitest's jsdom environment it shadows
// jsdom's own implementation, and the object it provides has no Storage
// methods, so any test touching localStorage dies with
// "localStorage.clear is not a function".
//
// This must be set via NODE_OPTIONS rather than a CLI flag: vitest runs tests
// in worker processes, and NODE_OPTIONS is inherited by children while a flag
// passed to the parent is not. Setting it here rather than inline in the npm
// script keeps `npm run test` working on Windows, where cmd.exe does not
// understand `VAR=value command` syntax.
// Guarded on the global actually existing rather than on a version number:
// older Node (CI runs 20) rejects the flag outright with "--no-experimental-
// webstorage is not allowed in NODE_OPTIONS", which kills every vitest worker.
// If this process has no built-in localStorage there is nothing to disable.
const WEBSTORAGE_OFF = "--no-experimental-webstorage"
const hasBuiltinWebStorage = typeof (globalThis as { localStorage?: unknown }).localStorage !== "undefined"
if (hasBuiltinWebStorage && !(process.env.NODE_OPTIONS ?? "").includes(WEBSTORAGE_OFF)) {
  process.env.NODE_OPTIONS = `${process.env.NODE_OPTIONS ?? ""} ${WEBSTORAGE_OFF}`.trim()
}

export default defineConfig({
  test: {
    environment: "jsdom",
    globals: true,
  },
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
})

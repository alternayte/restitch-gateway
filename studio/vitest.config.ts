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
const NODE_25_WEBSTORAGE_OFF = "--no-experimental-webstorage"
if (!(process.env.NODE_OPTIONS ?? "").includes(NODE_25_WEBSTORAGE_OFF)) {
  process.env.NODE_OPTIONS = `${process.env.NODE_OPTIONS ?? ""} ${NODE_25_WEBSTORAGE_OFF}`.trim()
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

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
//
// The switch keys off the Node major version, not on the presence of the
// global: Node 26 does not install the built-in localStorage until something
// accesses it, so a `typeof globalThis.localStorage` probe reads "undefined"
// at config load time even though workers still receive the broken global.
// Older Node (CI runs 24 and the floor is declared in .github/workflows)
// rejects the flag outright with "--no-experimental-webstorage is not allowed
// in NODE_OPTIONS", which kills every vitest worker, so the flag is set only
// on the majors that know it.
const WEBSTORAGE_OFF = "--no-experimental-webstorage"
const nodeMajor = Number(process.versions.node.split(".")[0])
if (nodeMajor >= 25 && !(process.env.NODE_OPTIONS ?? "").includes(WEBSTORAGE_OFF)) {
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

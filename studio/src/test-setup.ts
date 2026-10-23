import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
import "@/api-client";
import { client } from "@/api/client.gen";

// Node's Request needs an absolute URL; a browser resolves the relative one.
client.setConfig({ baseUrl: "http://localhost:3000" });

// jsdom has no modal dialog support; the Dialog component opens the native element.
if (typeof HTMLDialogElement !== "undefined" && !HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function (this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function (this: HTMLDialogElement) {
    this.open = false;
    this.dispatchEvent(new Event("close"));
  };
}

// React Flow measures its nodes; jsdom has no layout, so these only need to exist.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
globalThis.DOMMatrixReadOnly ??= class {
  m22 = 1;
} as unknown as typeof DOMMatrixReadOnly;

// Without vitest globals, Testing Library does not unmount between tests.
afterEach(() => cleanup());

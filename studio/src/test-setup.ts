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

// Without vitest globals, Testing Library does not unmount between tests.
afterEach(() => cleanup());

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

afterEach(() => vi.unstubAllGlobals());

function renderAt(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({ routeTree, context: { queryClient }, history: createMemoryHistory({ initialEntries: [path] }) });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

function stub(preview: unknown, onPublish: (body: unknown) => void) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const url = input.url;
      if (url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "e@x", role: "editor" });
      if (url.endsWith("/publish/preview")) return json(200, preview);
      if (url.endsWith("/publish")) {
        onPublish(await input.json());
        return json(200, { number: 2, author: "e@x", message: "m", created_at: "2026-10-24T00:00:00Z", yaml: "" });
      }
      if (url.endsWith("/versions"))
        return json(200, { versions: [{ number: 1, author: "e@x", message: "first", created_at: "2026-10-23T00:00:00Z" }] });
      if (url.endsWith("/api/v1/environments")) return json(200, { environments: [{ name: "staging" }, { name: "prod" }] });
      return json(404, { title: "Not Found" });
    }),
  );
}

describe("the publish screen", () => {
  it("shows validation errors and blocks publish", async () => {
    stub({ valid: false, errors: ["composition orders is not public and the artifact has no auth policy"], yaml: "", diff: "", current_version: 1 }, () => {
      throw new Error("must not publish");
    });
    renderAt("/environments/staging/versions");
    fireEvent.click(await screen.findByRole("button", { name: "Review changes" }));
    expect(await screen.findByText(/no auth policy/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish" })).toBeDisabled();
  });

  it("publishes a valid draft with a message", async () => {
    const published: unknown[] = [];
    stub(
      { valid: true, errors: [], yaml: "upstreams: {}\n", diff: "--- version 1\n+++ draft\n@@ -1 +1 @@\n-a\n+b\n", current_version: 1 },
      (b) => published.push(b),
    );
    renderAt("/environments/staging/versions");
    expect(await screen.findByText("first")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Review changes" }));
    fireEvent.change(await screen.findByLabelText("Message"), { target: { value: "Raise the timeout" } });
    fireEvent.click(screen.getByRole("button", { name: "Publish" }));
    await waitFor(() => expect(published).toEqual([{ message: "Raise the timeout" }]));
  });
});

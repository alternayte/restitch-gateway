import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
afterEach(() => vi.unstubAllGlobals());

const resource = {
  kind: "upstream",
  name: "users-api",
  body: {},
  revision: 1,
  updated_at: "2026-10-24T00:00:00Z",
  updated_by: "e@x",
};

function stubApi(role: string) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const path = new URL(input.url).pathname;
      if (path === "/api/v1/me") return json(200, { id: "u", email: "e@x", role });
      if (path === "/api/v1/environments")
        return json(200, { environments: [{ name: "prod", created_at: "2026-10-24T00:00:00Z", created_by: "o@x" }] });
      if (path === "/api/v1/environments/prod/resources") return json(200, { resources: [resource] });
      return json(404, { title: "Not Found", status: 404 });
    }),
  );
}

function renderAt(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({ routeTree, context: { queryClient }, history: createMemoryHistory({ initialEntries: [path] }) });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

async function openPalette() {
  await screen.findByRole("heading", { name: "Environments" });
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  return screen.findByRole("combobox", { name: "Search or run a command" });
}

describe("command palette", () => {
  it("finds a resource by name", async () => {
    stubApi("editor");
    const router = renderAt("/");
    const input = await openPalette();
    fireEvent.change(input, { target: { value: "users" } });
    const results = screen.getByRole("listbox", { name: "Results" });
    await within(results).findByRole("option", { name: /users-api/ });
    // Enter opens the best match, not a weaker fuzzy match from an earlier group.
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(router.state.location.pathname).toBe("/environments/prod/resources/upstream/users-api"));
  });

  it("hides owner-only pages and actions from viewers", async () => {
    stubApi("viewer");
    renderAt("/");
    const input = await openPalette();
    fireEvent.change(input, { target: { value: "users" } });
    const results = screen.getByRole("listbox", { name: "Results" });
    await within(results).findByRole("option", { name: /users-api/ });
    expect(within(results).queryByRole("option", { name: /^Users/ })).not.toBeInTheDocument();
    fireEvent.change(input, { target: { value: "new" } });
    expect(within(results).queryByRole("option", { name: /New environment/ })).not.toBeInTheDocument();
    expect(within(results).queryByRole("option", { name: /New resource/ })).not.toBeInTheDocument();
  });
});

describe("keyboard shortcuts", () => {
  it("jumps to a page with g and a letter", async () => {
    stubApi("owner");
    const router = renderAt("/");
    await screen.findByRole("heading", { name: "Environments" });
    fireEvent.keyDown(window, { key: "g" });
    fireEvent.keyDown(window, { key: "u" });
    await waitFor(() => expect(router.state.location.pathname).toBe("/settings/users"));
  });

  it("jumps between the pages of the current environment", async () => {
    stubApi("editor");
    const router = renderAt("/environments/prod/draft");
    await screen.findByRole("link", { name: "users-api" });
    fireEvent.keyDown(window, { key: "g" });
    fireEvent.keyDown(window, { key: "p" });
    await waitFor(() => expect(router.state.location.pathname).toBe("/environments/prod/versions"));
  });

  it("shows the shortcut sheet on ?", async () => {
    stubApi("editor");
    renderAt("/");
    await screen.findByRole("heading", { name: "Environments" });
    fireEvent.keyDown(window, { key: "?" });
    expect(await screen.findByRole("heading", { name: "Keyboard shortcuts" })).toBeInTheDocument();
  });

  it("opens the palette again after Escape closed it", async () => {
    stubApi("editor");
    renderAt("/");
    await openPalette();
    const palette = screen.getByRole("dialog", { name: "Command palette", hidden: true });
    // Escape sends a cancel event. Chromium does not always follow it with a close event.
    fireEvent(palette, new Event("cancel", { cancelable: true }));
    await waitFor(() => expect(palette).not.toHaveAttribute("open"));
    fireEvent.click(screen.getAllByRole("button", { name: /Search|Open the command palette/ })[0]!);
    await waitFor(() => expect(palette).toHaveAttribute("open"));
  });
});

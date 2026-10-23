import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@/api-client";
import { routeTree } from "@/routeTree.gen";

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

function respond(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

afterEach(() => vi.unstubAllGlobals());

describe("the app shell", () => {
  it("redirects to sign-in without a session", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => respond(401, { error: { code: "UNAUTHORIZED", message: "Sign in first." } })),
    );
    const router = renderAt("/");
    expect(await screen.findByRole("heading", { name: "Sign in to Restitch Studio" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/login");
  });

  it("lists environments for a signed-in user", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string) => {
        const url = typeof input === "string" ? input : input.url;
        if (url.endsWith("/api/v1/me")) return respond(200, { id: "u1", email: "owner@example.com", role: "owner" });
        if (url.endsWith("/api/v1/environments"))
          return respond(200, {
            environments: [{ name: "staging", created_at: "2026-10-23T10:00:00Z", created_by: "owner@example.com" }],
          });
        return respond(404, { title: "Not Found" });
      }),
    );
    renderAt("/");
    expect(await screen.findByText("staging")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /New environment/ })).toBeInTheDocument();
  });
});

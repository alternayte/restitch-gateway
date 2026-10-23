import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { fireEvent, render, screen } from "@testing-library/react";
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

const issued = { id: "t1", name: "gw", prefix: "rgt_abcdef", created_at: "2026-10-24T00:00:00Z", created_by: "o@x", token: "rgt_abcdef-the-whole-secret" };

describe("gateway tokens", () => {
  it("shows a new token once", async () => {
    let listed: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "o@x", role: "owner" });
        if (input.method === "POST") {
          listed = [{ ...issued, token: undefined }];
          return json(200, issued);
        }
        return json(200, { tokens: listed });
      }),
    );
    renderAt("/environments/prod/tokens");
    fireEvent.click(await screen.findByRole("button", { name: /New token/ }));
    fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "gw" } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByDisplayValue(issued.token)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(await screen.findByText("rgt_abcdef…")).toBeInTheDocument();
    expect(screen.queryByDisplayValue(issued.token)).not.toBeInTheDocument();
  });

  it("hides token management from editors", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) =>
        input.url.endsWith("/api/v1/me") ? json(200, { id: "u", email: "e@x", role: "editor" }) : json(200, { tokens: [] }),
      ),
    );
    renderAt("/environments/prod/tokens");
    expect(await screen.findByText(/Only owners manage gateway tokens/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /New token/ })).not.toBeInTheDocument();
  });
});

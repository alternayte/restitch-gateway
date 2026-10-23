import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
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

function stub(routes: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request) => {
      const path = new URL(input.url).pathname;
      if (path === "/api/v1/me") return json(200, { id: "u", email: "o@x", role: "owner" });
      for (const [suffix, body] of Object.entries(routes)) if (path.endsWith(suffix)) return json(200, body);
      return json(404, { title: "Not Found" });
    }),
  );
}

describe("the environment overview", () => {
  it("shows the empty state before any gateway reports", async () => {
    stub({
      "/instances": { current_version: 0, instances: [] },
      "/usage": { latency_bounds_ms: [10, 50], buckets: [], upstreams: [] },
      "/requests": { requests: [] },
    });
    renderAt("/environments/prod");
    expect(await screen.findByText(/No gateway has reported yet/)).toBeInTheDocument();
    expect(screen.getByText(/RESTITCH_GATEWAY_TOKEN/)).toBeInTheDocument();
  });

  it("shows instances, totals and failed requests", async () => {
    stub({
      "/instances": {
        current_version: 3,
        instances: [
          { instance_id: "gw-a", binary_version: "v2.0.0", applied_version: 3, stale: false, first_seen_at: "2026-10-24T00:00:00Z", last_seen_at: "2026-10-24T00:00:00Z" },
          { instance_id: "gw-b", binary_version: "v2.0.0", applied_version: 2, stale: true, first_seen_at: "2026-10-24T00:00:00Z", last_seen_at: "2026-10-24T00:00:00Z", last_pull_error: "studio returned status 502" },
        ],
      },
      "/usage": {
        latency_bounds_ms: [10, 50],
        buckets: [
          { timestamp: "2026-10-24T00:00:00Z", requests: 90, errors: 9, partials: 0, latency_buckets: [80, 10, 0], p95_ms: 50, steps: {} },
          { timestamp: "2026-10-24T00:01:00Z", requests: 10, errors: 1, partials: 0, latency_buckets: [10, 0, 0], p95_ms: 10, steps: {} },
        ],
        upstreams: [{ name: "users", requests: 95, errors: 10, breakers: { "gw-a": "open" } }],
      },
      "/requests": {
        requests: [{ id: "r1", instance_id: "gw-a", composition: "user", method: "GET", path: "/users/1", status: 502, duration_ms: 12, partial: false, time: "2026-10-24T00:00:30Z", steps: [] }],
      },
    });
    renderAt("/environments/prod");
    expect(await screen.findByText("gw-b")).toBeInTheDocument();
    expect(screen.getByText("studio returned status 502")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("10.0%")).toBeInTheDocument();
    expect(screen.getByText("open on gw-a")).toBeInTheDocument();
    expect(screen.getByText("/users/1")).toBeInTheDocument();
  });
});

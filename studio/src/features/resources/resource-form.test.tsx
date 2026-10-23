import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { routeTree } from "@/routeTree.gen";

const schema = {
  artifact: {
    $defs: {
      "composition.Upstream": {
        type: "object",
        additionalProperties: false,
        properties: {
          url: { type: "string" },
          timeout: { type: "string", pattern: "^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$" },
          retry: { $ref: "#/$defs/composition.RetryConfig" },
        },
      },
      "composition.RetryConfig": {
        type: "object",
        additionalProperties: false,
        properties: { max_attempts: { type: "integer" } },
      },
    },
  },
  definitions: { upstream: "composition.Upstream" },
  settings: {},
};

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

describe("the resource form", () => {
  it("saves an upstream created through the form", async () => {
    const puts: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "e@x", role: "editor" });
        if (input.url.endsWith("/api/v1/schema")) return json(200, schema);
        if (input.method === "PUT") {
          const body = (await input.json()) as { body: Record<string, unknown> };
          puts.push(body);
          return json(200, { kind: "upstream", name: "users", revision: 1, body: body.body, updated_at: "2026-10-24T00:00:00Z", updated_by: "e@x" });
        }
        if (input.url.endsWith("/resources/upstream/users")) return json(404, { title: "Not Found", detail: "upstream \"users\" not found" });
        return json(200, { resources: [] });
      }),
    );
    renderAt("/environments/staging/resources/upstream/users");

    fireEvent.change(await screen.findByLabelText("Url"), { target: { value: "https://${USERS_HOST}" } });
    fireEvent.change(screen.getByLabelText("Timeout"), { target: { value: "5x" } });
    expect(screen.getByLabelText("Timeout")).toHaveAttribute("aria-invalid", "true");
    fireEvent.change(screen.getByLabelText("Timeout"), { target: { value: "5s" } });
    fireEvent.click(screen.getByRole("button", { name: "Add Retry" }));
    fireEvent.change(screen.getByLabelText("Max attempts"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]).toEqual({ base_revision: 0, body: { url: "https://${USERS_HOST}", timeout: "5s", retry: { max_attempts: 3 } } });
    expect(await screen.findByText("Revision 1 in the staging draft.")).toBeInTheDocument();
  });

  it("offers a reload when the save is stale", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "e@x", role: "editor" });
        if (input.url.endsWith("/api/v1/schema")) return json(200, schema);
        if (input.method === "PUT") return json(409, { title: "Conflict", status: 409, detail: "changed since you loaded it" });
        return json(200, { kind: "upstream", name: "users", revision: 4, body: { url: "https://a" }, updated_at: "2026-10-24T00:00:00Z", updated_by: "x" });
      }),
    );
    renderAt("/environments/staging/resources/upstream/users");
    fireEvent.change(await screen.findByLabelText("Url"), { target: { value: "https://b" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByRole("button", { name: "Reload" })).toBeInTheDocument();
  });
});

describe("template fields", () => {
  it("show the lint error from Studio", async () => {
    const compositionSchema = {
      artifact: {
        $defs: {
          "composition.Composition": {
            type: "object",
            additionalProperties: false,
            properties: { path: { type: "string" }, steps: { type: "array", items: { $ref: "#/$defs/composition.Step" } } },
          },
          "composition.Step": {
            type: "object",
            additionalProperties: false,
            properties: { name: { type: "string" }, path: { type: "string" } },
          },
        },
      },
      definitions: { composition: "composition.Composition" },
      settings: {},
    };
    const linted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "e@x", role: "editor" });
        if (input.url.endsWith("/api/v1/schema")) return json(200, compositionSchema);
        if (input.url.endsWith("/api/v1/lint")) {
          linted.push(await input.json());
          return json(200, { errors: ['expression "req.path.id": req.path is a string and has no member "id"'] });
        }
        return json(200, { kind: "composition", name: "c", revision: 1, body: { steps: [{ name: "u" }] }, updated_at: "2026-10-24T00:00:00Z", updated_by: "x" });
      }),
    );
    renderAt("/environments/staging/resources/composition/c");
    const paths = await screen.findAllByLabelText("Path");
    fireEvent.change(paths[1]!, { target: { value: "/x/{{ req.path.id }}" } });
    expect(await screen.findByText(/req.path is a string/)).toBeInTheDocument();
    expect(linted).toEqual([{ template: "/x/{{ req.path.id }}", steps: ["u"] }]);
  });
});

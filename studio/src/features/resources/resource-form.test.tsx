import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

describe("the secret picker", () => {
  it("inserts a ${NAME} at the cursor and refuses an invalid new name", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "e@x", role: "editor" });
        if (input.url.endsWith("/api/v1/schema")) return json(200, schema);
        if (input.url.endsWith("/secrets"))
          return json(200, { names: [{ name: "USERS_HOST", optional: false, used_by: ["upstream/users"] }] });
        return json(200, { kind: "upstream", name: "users", revision: 1, body: { url: "https:///v1" }, updated_at: "2026-10-24T00:00:00Z", updated_by: "x" });
      }),
    );
    renderAt("/environments/staging/resources/upstream/users");
    const url = (await screen.findByLabelText("Url")) as HTMLInputElement;
    url.setSelectionRange(8, 8);
    fireEvent.click(screen.getByRole("button", { name: "Insert a secret name into Url" }));
    fireEvent.click(await screen.findByRole("button", { name: "${USERS_HOST}" }));
    expect(url.value).toBe("https://${USERS_HOST}/v1");
    expect(url.selectionStart).toBe(21);

    fireEvent.click(screen.getByRole("button", { name: "Insert a secret name into Url" }));
    fireEvent.change(screen.getByLabelText("Secret name"), { target: { value: "1BAD" } });
    expect(screen.getByRole("alert")).toHaveTextContent("do not start with a digit");
    fireEvent.change(screen.getByLabelText("Secret name"), { target: { value: "API_TOKEN" } });
    fireEvent.click(screen.getByRole("button", { name: /Insert new name/ }));
    expect(url.value).toBe("https://${USERS_HOST}${API_TOKEN}/v1");
  });
});

describe("the secret picker allowlist", () => {
  const pickerSchema = {
    artifact: {
      $defs: {
        "composition.Upstream": {
          type: "object",
          additionalProperties: false,
          properties: { url: { type: "string" }, auth: { $ref: "#/$defs/auth.Config" } },
        },
        "auth.Config": {
          type: "object",
          additionalProperties: false,
          properties: { header: { $ref: "#/$defs/auth.HeaderConfig" } },
        },
        "auth.HeaderConfig": {
          type: "object",
          additionalProperties: false,
          properties: { name: { type: "string" }, value: { type: "string" } },
        },
        "composition.Composition": {
          type: "object",
          additionalProperties: false,
          properties: { path: { type: "string" }, steps: { type: "array", items: { $ref: "#/$defs/composition.Step" } } },
        },
        "composition.Step": {
          type: "object",
          additionalProperties: false,
          properties: {
            name: { type: "string" },
            upstream: { type: "string" },
            method: { type: "string" },
            path: { type: "string" },
            headers: { type: "object", additionalProperties: { type: "string" } },
          },
        },
      },
    },
    definitions: { upstream: "composition.Upstream", composition: "composition.Composition" },
    settings: {},
  };
  const stub = (body: unknown) =>
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.url.endsWith("/api/v1/me")) return json(200, { id: "u", email: "e@x", role: "editor" });
        if (input.url.endsWith("/api/v1/schema")) return json(200, pickerSchema);
        if (input.url.endsWith("/secrets")) return json(200, { names: [] });
        if (input.url.endsWith("/api/v1/lint")) return json(200, { errors: [] });
        return json(200, { kind: "x", name: "x", revision: 1, body, updated_at: "2026-10-24T00:00:00Z", updated_by: "x" });
      }),
    );
  const picker = (field: string) => screen.queryByRole("button", { name: `Insert a secret name into ${field}` });

  it("shows the secret picker on an auth value but not on a step name", async () => {
    stub({ url: "https://a", auth: { header: { name: "Authorization", value: "" } } });
    renderAt("/environments/staging/resources/upstream/users");
    await screen.findByLabelText("Value");
    expect(picker("Url")).toBeInTheDocument();
    expect(picker("Value")).toBeInTheDocument();
    expect(picker("Name")).not.toBeInTheDocument();
    cleanup();

    stub({ path: "/users", steps: [{ name: "u", upstream: "users", method: "GET", path: "/v1", headers: { "X-Key": "" } }] });
    renderAt("/environments/staging/resources/composition/c");
    await screen.findByLabelText("Upstream");
    expect(picker("Name")).not.toBeInTheDocument();
    expect(picker("Upstream")).not.toBeInTheDocument();
    expect(picker("Method")).not.toBeInTheDocument();
    // The inbound route path is not a secret; the step path is.
    expect(screen.getAllByRole("button", { name: "Insert a secret name into Path" })).toHaveLength(1);
    expect(picker("Headers X-Key")).toBeInTheDocument();
  });
});

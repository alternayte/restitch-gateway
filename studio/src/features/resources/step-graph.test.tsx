import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { stepEdges } from "@/features/resources/step-edges";
import { StepGraph, type Step } from "@/features/resources/step-graph";

function connect(step: string, dependsOn: string) {
  fireEvent.change(screen.getByLabelText("Step"), { target: { value: step } });
  fireEvent.change(screen.getByLabelText("Depends on"), { target: { value: dependsOn } });
  fireEvent.click(screen.getByRole("button", { name: "Add edge" }));
}

describe("the step graph", () => {
  it("refuses an edge that makes a cycle", () => {
    const onChange = vi.fn();
    const steps: Step[] = [{ name: "a" }, { name: "b", depends_on: ["a"] }, { name: "c", depends_on: ["b"] }];
    render(<StepGraph steps={steps} onChange={onChange} />);

    connect("a", "c");

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("a cannot depend on c: c already depends on a.");
  });

  it("adds depends_on when two steps are connected", () => {
    const onChange = vi.fn();
    const steps: Step[] = [{ name: "a", upstream: "u" }, { name: "b", upstream: "u", depends_on: ["a"] }, { name: "c" }];
    render(<StepGraph steps={steps} onChange={onChange} />);

    connect("c", "b");

    expect(onChange).toHaveBeenCalledWith([
      { name: "a", upstream: "u" },
      { name: "b", upstream: "u", depends_on: ["a"] },
      { name: "c", depends_on: ["b"] },
    ]);
  });

  it("removes depends_on when the edge is removed", () => {
    const onChange = vi.fn();
    const steps: Step[] = [{ name: "a" }, { name: "b", path: "/b", depends_on: ["a"] }];
    render(<StepGraph steps={steps} onChange={onChange} />);

    fireEvent.click(screen.getByRole("button", { name: "Remove edge a to b" }));

    expect(onChange).toHaveBeenCalledWith([{ name: "a" }, { name: "b", path: "/b" }]);
  });

  it("draws template references as dashed edges that cannot be removed", () => {
    const steps: Step[] = [{ name: "user" }, { name: "orders", path: "/orders/{{ steps.user.body.id }}" }];
    render(<StepGraph steps={steps} inferred={{ orders: ["user"] }} onChange={vi.fn()} />);

    expect(screen.getByRole("list", { name: "Edges" })).toHaveTextContent("orders uses user in a template");
    expect(screen.queryByRole("button", { name: "Remove edge user to orders" })).toBeNull();
    expect(screen.getByText(/Dashed edges come from template references/)).toBeInTheDocument();

    const edge = stepEdges([{ name: "user", depends_on: [] }, { name: "orders", depends_on: [] }], { orders: ["user"] }, new Set())[0];
    expect(edge).toMatchObject({ source: "user", target: "orders", deletable: false, selectable: false });
    expect(edge?.style?.strokeDasharray).toBeTruthy();
  });

  it("does not draw a template reference twice when depends_on names it too", () => {
    const steps: Step[] = [{ name: "a" }, { name: "b", depends_on: ["a"] }];
    render(<StepGraph steps={steps} inferred={{ b: ["a"] }} onChange={vi.fn()} />);

    expect(screen.getByRole("list", { name: "Edges" }).children).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Remove edge a to b" })).toBeInTheDocument();
  });

  it("refuses an edge that closes a cycle through a template reference", () => {
    const onChange = vi.fn();
    // b depends on a through depends_on; c uses b in a template.
    const steps: Step[] = [{ name: "a" }, { name: "b", depends_on: ["a"] }, { name: "c", path: "/{{ steps.b.body.id }}" }];
    render(<StepGraph steps={steps} inferred={{ c: ["b"] }} onChange={onChange} />);

    connect("a", "c");

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent("a cannot depend on c: c already depends on a.");
  });

  it("shows why the gateway refuses the steps", () => {
    render(<StepGraph steps={[{ name: "a" }]} problem="circular dependency detected involving steps: [a b]" onChange={vi.fn()} />);

    expect(screen.getByText(/circular dependency detected/)).toBeInTheDocument();
  });
});

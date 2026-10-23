import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
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
});

import { createContext, useContext, useEffect, useState } from "react";
import { lintTemplate, stepDependencies } from "@/api/sdk.gen";

/** StepDeps is what Studio's compiler reports for the steps being edited. */
export type StepDeps = { inferred: Record<string, string[]>; problem: string };

const noDeps: StepDeps = { inferred: {}, problem: "" };

/**
 * useStepDependencies asks Studio which steps each step's templates reference,
 * 400ms after steps stops changing. steps is the unsaved steps list; null
 * means the resource is not a composition and nothing is sent.
 */
export function useStepDependencies(steps: unknown): StepDeps {
  const [deps, setDeps] = useState<StepDeps>(noDeps);
  // A new array arrives on every render; its JSON decides when to ask again.
  const key = steps === null ? null : JSON.stringify(steps ?? []);

  useEffect(() => {
    // The editor remounts for another resource, so key never turns null mid-edit.
    if (key === null) return;
    let live = true;
    const timer = setTimeout(() => {
      stepDependencies({ body: { composition: { steps: JSON.parse(key) as unknown } } })
        .then(({ data }) => {
          if (!live) return;
          if (!data) {
            setDeps(noDeps);
            return;
          }
          const inferred: Record<string, string[]> = {};
          for (const [name, d] of Object.entries(data.steps)) inferred[name] = d.inferred ?? [];
          setDeps({ inferred, problem: data.error });
        })
        .catch(() => {
          if (live) setDeps(noDeps);
        });
    }, 400);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [key]);

  return deps;
}

/** StepNames gives template fields the step names of the composition they belong to. */
export const StepNames = createContext<string[] | null>(null);

/** useTemplateLint asks Studio to check value 400ms after it stops changing. Plain text is never sent. */
export function useTemplateLint(value: string): string {
  const steps = useContext(StepNames);
  const [error, setError] = useState("");
  const isTemplate = value.includes("{{");
  // A new array arrives on every render; the names decide when to re-check.
  const stepKey = steps === null ? null : steps.join("\u0000");

  useEffect(() => {
    if (!isTemplate || stepKey === null) {
      setError("");
      return;
    }
    let live = true;
    const timer = setTimeout(() => {
      lintTemplate({ body: { template: value, steps: stepKey === "" ? [] : stepKey.split("\u0000") } })
        .then(({ data }) => {
          if (live) setError(data?.errors?.[0] ?? "");
        })
        .catch(() => {
          if (live) setError("");
        });
    }, 400);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [value, isTemplate, stepKey]);

  return error;
}

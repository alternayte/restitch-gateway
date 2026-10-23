import { createContext, useContext, useEffect, useState } from "react";
import { lintTemplate } from "@/api/sdk.gen";

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

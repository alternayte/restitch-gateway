import { ApiError } from "@/api-client";

/** errorMessage returns a message for the user from an API or other error. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429) return "Too many attempts. Wait and try again.";
    if (err.code === "INSUFFICIENT_ROLE") return "Your role cannot do this.";
    return err.message || "Request failed.";
  }
  if (err instanceof Error) return err.message;
  return "Request failed.";
}

/** fieldErrors maps the problem details of a 422 to field messages, keyed by the last part of the location. */
export function fieldErrors(err: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (!(err instanceof ApiError) || !Array.isArray(err.details)) return out;
  for (const d of err.details as unknown[]) {
    if (d && typeof d === "object") {
      const { location, message } = d as { location?: unknown; message?: unknown };
      if (typeof location === "string" && typeof message === "string") {
        const field = location.split(".").pop() ?? location;
        if (!out[field]) out[field] = message;
      }
    }
  }
  return out;
}

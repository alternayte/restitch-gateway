import { client } from "@/api/client.gen";
import type { ErrorModel, ErrorResponse } from "@/api/types.gen";

/**
 * ApiError carries a failed response. Studio answers in two shapes: auth-all
 * routes send {"error":{"code","message"}}, the Studio API sends an RFC 7807
 * problem {"title","status","detail","errors"}. Both map onto this class.
 */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public details?: unknown,
  ) {
    super(message);
  }
}

const unauthorized = new Set<() => void>();

/** onUnauthorized registers a callback for 401 responses. */
export function onUnauthorized(fn: () => void) {
  unauthorized.add(fn);
  return () => unauthorized.delete(fn);
}

client.setConfig({ baseUrl: "", credentials: "same-origin", throwOnError: true });

client.interceptors.response.use((response) => {
  if (response.status === 401) unauthorized.forEach((fn) => fn());
  return response;
});

/**
 * toApiError converts a failed response body into an ApiError. It returns the original
 * error unchanged when there is no response, for example a network error from fetch itself,
 * so TanStack Query retries it.
 */
export function toApiError(error: unknown, response: Response | undefined): unknown {
  if (!response) return error;
  const auth = (error ?? {}) as Partial<ErrorResponse>;
  if (auth.error?.code) {
    return new ApiError(response.status, auth.error.code, auth.error.message ?? response.statusText);
  }
  const problem = (error ?? {}) as ErrorModel;
  return new ApiError(
    response.status,
    problem.title ?? "http_error",
    problem.detail || problem.title || response.statusText || "request failed",
    problem.errors ?? undefined,
  );
}

client.interceptors.error.use(toApiError);

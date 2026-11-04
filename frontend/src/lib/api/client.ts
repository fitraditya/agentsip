import { clearSession, getSession } from "./session";

/**
 * Base URL for the agentsip backend. Falls back to the local dev default so
 * `npm run dev` works out of the box against `../backend` running on 8080.
 */
export const API_BASE_URL: string =
  (import.meta.env.VITE_API_BASE_URL as string | undefined) ||
  "http://localhost:8080";

/** Dispatched on `window` whenever a request comes back 401. AuthContext
 * listens for this to clear its user state and navigate to /signin via the
 * router (instead of a hard page reload). */
export const UNAUTHORIZED_EVENT = "agentsip:unauthorized";

export type ApiErrorCode =
  | "NOT_FOUND"
  | "VALIDATION_FAILED"
  | "CONFLICT"
  | "TRUNK_LIMIT_EXCEEDED"
  | "UNAUTHORIZED"
  | "TRUNK_AT_CAPACITY"
  | "CALL_DECISION_ALREADY_MADE"
  | "EMAIL_ALREADY_REGISTERED"
  | "INTERNAL"
  | "NETWORK_ERROR";

/**
 * Thrown by every `apiRequest` call on failure. Callers get either the
 * unwrapped `data` payload or one of these — never the raw envelope.
 */
export class ApiError extends Error {
  /** One of `ApiErrorCode` in practice, but kept as `string` since the
   * backend is the source of truth and may add codes this client doesn't
   * know about yet. */
  readonly code: string;
  readonly status: number;
  readonly details?: Record<string, unknown>;

  constructor(
    message: string,
    code: string,
    status: number,
    details?: Record<string, unknown>,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.details = details;
  }
}

interface SuccessEnvelope<T> {
  success: true;
  data: T;
}

interface ErrorEnvelope {
  success: false;
  error_code: string;
  error_message: string;
  error_details?: Record<string, unknown>;
}

type Envelope<T> = SuccessEnvelope<T> | ErrorEnvelope;

interface ApiRequestOptions {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  /** Raw field object; automatically wrapped as `{"data": body}`. Omit for
   * GET/DELETE or for a POST that genuinely has no body. */
  body?: unknown;
  query?: Record<string, string | number | undefined>;
  /** Set false for the unauthenticated auth routes (register/login/etc).
   * Defaults to true — every other route requires `Authorization: Bearer`. */
  auth?: boolean;
}

function buildUrl(
  path: string,
  query?: ApiRequestOptions["query"],
): string {
  const base = API_BASE_URL.replace(/\/+$/, "");
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  const url = new URL(`${base}${normalizedPath}`);
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined) url.searchParams.set(key, String(value));
    }
  }
  return url.toString();
}

/**
 * Typed fetch wrapper around the agentsip backend envelope.
 *
 * - Wraps request bodies as `{"data": {...}}`.
 * - Unwraps `{"success": true, "data": {...}}` responses, returning `data`.
 * - Throws `ApiError` for `{"success": false, ...}` responses and for
 *   network failures.
 * - Attaches `Authorization: Bearer <token>` from the stored session unless
 *   `auth: false` is passed.
 * - On a 401, clears the stored session and dispatches `UNAUTHORIZED_EVENT`
 *   on `window` so a router-aware listener (AuthContext) can redirect.
 */
export async function apiRequest<T>(
  path: string,
  options: ApiRequestOptions = {},
): Promise<T> {
  const { method = "GET", body, query, auth = true } = options;

  const headers: Record<string, string> = {};
  let fetchBody: string | undefined;

  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    fetchBody = JSON.stringify({ data: body });
  }

  if (auth) {
    const session = getSession();
    if (session?.token) {
      headers.Authorization = `Bearer ${session.token}`;
    }
  }

  let response: Response;
  try {
    response = await fetch(buildUrl(path, query), {
      method,
      headers,
      body: fetchBody,
    });
  } catch {
    throw new ApiError(
      "Could not reach the backend. Is it running?",
      "NETWORK_ERROR",
      0,
    );
  }

  return unwrapResponse<T>(response);
}

/**
 * POSTs a `FormData` body (multipart/form-data) — for the one or two
 * endpoints that take a raw file upload rather than the JSON `{"data": ...}`
 * envelope every other route uses. Shares `apiRequest`'s auth header and
 * response envelope/error handling.
 */
export async function apiRequestFormData<T>(
  path: string,
  formData: FormData,
): Promise<T> {
  const headers: Record<string, string> = {};
  const session = getSession();
  if (session?.token) {
    headers.Authorization = `Bearer ${session.token}`;
  }

  let response: Response;
  try {
    // No Content-Type header: the browser sets multipart/form-data with
    // the correct boundary itself, which it can only do when it owns the
    // header.
    response = await fetch(buildUrl(path), {
      method: "POST",
      headers,
      body: formData,
    });
  } catch {
    throw new ApiError(
      "Could not reach the backend. Is it running?",
      "NETWORK_ERROR",
      0,
    );
  }

  return unwrapResponse<T>(response);
}

async function unwrapResponse<T>(response: Response): Promise<T> {
  let payload: Envelope<T> | undefined;
  try {
    payload = (await response.json()) as Envelope<T>;
  } catch {
    payload = undefined;
  }

  if (response.status === 401) {
    clearSession();
    window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT));
  }

  if (!payload || payload.success !== true) {
    const errorPayload =
      payload && payload.success === false ? payload : undefined;
    throw new ApiError(
      errorPayload?.error_message ??
        `Request failed with status ${response.status}`,
      errorPayload?.error_code ?? "INTERNAL",
      response.status,
      errorPayload?.error_details,
    );
  }

  return payload.data;
}

import { createSemliaClient } from "@semlia/sdk-typescript";


export const SESSION_INVALID_EVENT = "semlia:session-invalid";
export const AUTHORIZATION_STALE_EVENT = "semlia:authorization-stale";

let csrfToken = "";

export const apiClient = createSemliaClient({
  baseUrl: typeof window === "undefined" ? "http://localhost" : window.location.origin,
  fetch: (request) => globalThis.fetch(request),
  credentials: "include",
  getCSRFToken: () => csrfToken || undefined,
  onCSRFToken: (token) => {
    csrfToken = token;
  },
  onUnauthorized: (_response, request) => {
    dispatchBrowserEvent(SESSION_INVALID_EVENT, request.url);
  },
  onForbidden: (_response, request) => {
    dispatchBrowserEvent(AUTHORIZATION_STALE_EVENT, request.url);
  },
});

export function sessionCSRFToken(): string {
  return csrfToken;
}

export function clearSessionCSRFToken(): void {
  csrfToken = "";
}

function dispatchBrowserEvent(name: string, requestUrl: string): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent(name, { detail: { requestUrl } }));
}

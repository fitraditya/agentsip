import type { Session } from "./types";

/**
 * sessionStorage (not localStorage, deliberately — it's cleared when the tab
 * closes, which limits the blast radius of an XSS-exfiltrated token).
 */
export const SESSION_STORAGE_KEY = "agentsip.session";

export function getSession(): Session | null {
  try {
    const raw = sessionStorage.getItem(SESSION_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Session;
    if (!parsed?.token) return null;
    return parsed;
  } catch {
    return null;
  }
}

export function setSession(session: Session): void {
  try {
    sessionStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session));
  } catch {
    // sessionStorage unavailable (private mode, etc.) — fail open, the
    // request-level auth header simply won't be attached.
  }
}

export function clearSession(): void {
  try {
    sessionStorage.removeItem(SESSION_STORAGE_KEY);
  } catch {
    // ignore
  }
}

export function isSessionExpired(session: Session): boolean {
  const expiry = Date.parse(session.expires_at);
  if (Number.isNaN(expiry)) return false;
  return expiry <= Date.now();
}

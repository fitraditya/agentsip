import { authApi, UNAUTHORIZED_EVENT } from "@/lib/api";
import { clearSession, getSession, isSessionExpired } from "@/lib/api/session";
import type { User } from "@/lib/api/types";
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useNavigate } from "react-router";

interface AuthContextType {
  /** The signed-in user's profile. `null` only before the initial
   * sessionStorage/`/me` bootstrap on mount resolves — see `isInitializing`.
   * See `isAuthenticated` for the actual auth signal. */
  user: User | null;
  /** True whenever a non-expired session token is stored, independent of
   * whether `user` has been rehydrated. This is what route guards check. */
  isAuthenticated: boolean;
  /** True until the sessionStorage bootstrap check on mount has run. */
  isInitializing: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Re-fetches `/me` and updates `user` — call after changing the email on
   * the profile page. */
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [hasSession, setHasSession] = useState(false);
  const [isInitializing, setIsInitializing] = useState(true);
  const navigate = useNavigate();

  // Bootstrap: check sessionStorage once on mount. If the stored session has
  // already passed its expires_at, drop it proactively instead of waiting
  // for the first 401. A still-valid session rehydrates the user profile via
  // /me, so a page refresh doesn't lose it.
  useEffect(() => {
    const session = getSession();
    if (session && isSessionExpired(session)) {
      clearSession();
      setHasSession(false);
      setIsInitializing(false);
      return;
    }

    setHasSession(session !== null);
    if (session === null) {
      setIsInitializing(false);
      return;
    }

    authApi
      .me()
      .then(setUser)
      .catch(() => {
        // The token was rejected server-side (expired/revoked) — the
        // apiRequest client already cleared it and dispatched
        // UNAUTHORIZED_EVENT, which the handler below reacts to.
      })
      .finally(() => setIsInitializing(false));
  }, []);

  // The API client dispatches this on any 401 response after clearing the
  // stored session. Mirror that here and send the user to sign-in.
  useEffect(() => {
    const handleUnauthorized = () => {
      setUser(null);
      setHasSession(false);
      void navigate("/signin", { replace: true });
    };
    window.addEventListener(UNAUTHORIZED_EVENT, handleUnauthorized);
    return () =>
      window.removeEventListener(UNAUTHORIZED_EVENT, handleUnauthorized);
  }, [navigate]);

  const login = async (email: string, password: string) => {
    const { user: loggedInUser } = await authApi.login(email, password);
    setUser(loggedInUser);
    setHasSession(true);
  };

  const logout = async () => {
    await authApi.logout().catch(() => {
      // Token already invalid server-side — local state is cleared below
      // regardless, so this is not a user-facing failure.
    });
    setUser(null);
    setHasSession(false);
    void navigate("/signin", { replace: true });
  };

  const refreshUser = async () => {
    const refreshed = await authApi.me();
    setUser(refreshed);
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated: hasSession,
        isInitializing,
        login,
        logout,
        refreshUser,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

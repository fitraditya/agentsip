import { apiRequest } from "./client";
import { clearSession, setSession } from "./session";
import type {
  ChangePasswordRequest,
  LoginResult,
  UpdateEmailRequest,
  User,
} from "./types";

interface RegisterResponse {
  user: User;
}

interface LoginResponse {
  user: User;
  session: LoginResult["session"];
}

export const authApi = {
  /**
   * Creates the user only — no session/token is returned. Call `login`
   * separately afterward. Can throw `EMAIL_ALREADY_REGISTERED` (409).
   */
  async register(email: string, password: string): Promise<User> {
    const data = await apiRequest<RegisterResponse>("/register", {
      method: "POST",
      body: { email, password },
      auth: false,
    });
    return data.user;
  },

  /** Stores the returned session in sessionStorage and returns it alongside the user. */
  async login(email: string, password: string): Promise<LoginResult> {
    const data = await apiRequest<LoginResponse>("/login", {
      method: "POST",
      body: { email, password },
      auth: false,
    });
    setSession(data.session);
    return data;
  },

  /** Always clears the local session, even if the server call fails
   * (e.g. the token was already expired/revoked). */
  async logout(): Promise<void> {
    try {
      await apiRequest<{ logged_out: boolean }>("/logout", { method: "POST" });
    } finally {
      clearSession();
    }
  },

  async verifyEmail(token: string): Promise<boolean> {
    const data = await apiRequest<{ verified: boolean }>("/verify-email", {
      method: "POST",
      body: { token },
      auth: false,
    });
    return data.verified;
  },

  /** Always resolves with the same enumeration-safe message, regardless of
   * whether the email exists. */
  async resendVerification(email: string): Promise<string> {
    const data = await apiRequest<{ message: string }>(
      "/resend-verification",
      { method: "POST", body: { email }, auth: false },
    );
    return data.message;
  },

  /** Enumeration-safe: always resolves with the same message. */
  async requestPasswordReset(email: string): Promise<string> {
    const data = await apiRequest<{ message: string }>(
      "/password-reset/request",
      { method: "POST", body: { email }, auth: false },
    );
    return data.message;
  },

  async confirmPasswordReset(
    token: string,
    newPassword: string,
  ): Promise<boolean> {
    const data = await apiRequest<{ password_reset: boolean }>(
      "/password-reset/confirm",
      {
        method: "POST",
        body: { token, new_password: newPassword },
        auth: false,
      },
    );
    return data.password_reset;
  },

  /** Rehydrates the signed-in user's own profile from the stored session token. */
  async me(): Promise<User> {
    const data = await apiRequest<{ user: User }>("/me");
    return data.user;
  },

  /** Throws `UNAUTHORIZED` (401) if `current_password` is wrong, or
   * `EMAIL_ALREADY_REGISTERED` (409) if another user already holds the new
   * address. */
  async updateEmail(req: UpdateEmailRequest): Promise<User> {
    const data = await apiRequest<{ user: User }>("/me/email", {
      method: "PUT",
      body: req,
    });
    return data.user;
  },

  /** Throws `UNAUTHORIZED` (401) if `current_password` is wrong. Does not
   * revoke the current session — it stays logged in. */
  async changePassword(req: ChangePasswordRequest): Promise<void> {
    await apiRequest<{ password_changed: boolean }>("/me/password", {
      method: "PUT",
      body: req,
    });
  },
};

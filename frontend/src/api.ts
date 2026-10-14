import type { Session } from "./types";

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
let token: string | null = null;
let expiresAt = 0;
let refreshPending: Promise<Session> | null = null;
export function clearSession() {
  token = null;
  expiresAt = 0;
  window.dispatchEvent(new Event("relations:logout"));
}
export function acceptSession(session: Session) {
  token = session.access_token;
  expiresAt = Date.now() + session.expires_in * 1000;
  window.dispatchEvent(
    new CustomEvent("relations:session", { detail: session.user }),
  );
  return session;
}
const withSessionLock = <T>(fn: () => Promise<T>): Promise<T> =>
  navigator.locks ? navigator.locks.request("relations-session", fn) : fn();

async function request<T>(
  path: string,
  init: RequestInit = {},
  authenticated = true,
): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body) headers.set("Content-Type", "application/json");
  if (authenticated && token) headers.set("Authorization", `Bearer ${token}`);
  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, {
      ...init,
      headers,
      credentials: "same-origin",
    });
  } catch {
    throw new ApiError(
      "network_error",
      "The service could not be reached. Please try again.",
      0,
    );
  }
  if (response.status === 204) return undefined as T;
  const data = await response.json().catch(() => ({}));
  if (!response.ok)
    throw new ApiError(
      data.error?.code || "request_failed",
      data.error?.message || "The request could not be completed.",
      response.status,
    );
  return data.data as T;
}
export function refresh(): Promise<Session> {
  if (!refreshPending) {
    refreshPending = withSessionLock(() =>
      request<Session>("/auth/refresh", { method: "POST" }, false).then(
        acceptSession,
      ),
    )
      .catch((error) => {
        if (error instanceof ApiError && error.status === 401) clearSession();
        throw error;
      })
      .finally(() => {
        refreshPending = null;
      });
  }
  return refreshPending;
}
export async function api<T>(
  path: string,
  init: RequestInit = {},
  authenticated = true,
): Promise<T> {
  if (authenticated && token && Date.now() >= expiresAt - 10000)
    await refresh();
  try {
    return await request<T>(path, init, authenticated);
  } catch (e) {
    if (authenticated && e instanceof ApiError && e.status === 401) {
      // A verified user's incorrect reauthentication password is not token expiry.
      if (
        path === "/users/me/password" ||
        (path === "/users/me" && init.method === "DELETE") ||
        e.code === "reauthentication_required"
      )
        throw e;
      await refresh();
      try {
        return await request<T>(path, init, authenticated);
      } catch (retryError) {
        if (retryError instanceof ApiError && retryError.status === 401)
          clearSession();
        throw retryError;
      }
    }
    throw e;
  }
}
export async function login(
  path: "/auth/login" | "/auth/register",
  body: unknown,
) {
  return withSessionLock(() =>
    request<Session>(
      path,
      { method: "POST", body: JSON.stringify(body) },
      false,
    ).then(acceptSession),
  );
}
export async function logout(all = false) {
  const finish = () =>
    withSessionLock(async () => {
      await request(
        all ? "/auth/logout-all" : "/auth/logout",
        { method: "POST" },
        all,
      );
      clearSession();
      const channel =
        typeof BroadcastChannel !== "undefined"
          ? new BroadcastChannel("relations-auth")
          : null;
      channel?.postMessage("logout");
      channel?.close();
    });
  try {
    await finish();
  } catch (e) {
    if (all && e instanceof ApiError && e.status === 401) {
      await refresh();
      await finish();
    } else throw e;
  }
}
export const json = (method: string, body: unknown): RequestInit => ({
  method,
  body: JSON.stringify(body),
});

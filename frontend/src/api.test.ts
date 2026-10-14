import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const session = {
  access_token: "memory-only-access",
  expires_in: 900,
  user: { id: "1", name: "Reader" },
};
const reply = (data: unknown, status = 200) =>
  new Response(JSON.stringify(status >= 400 ? { error: data } : { data }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
describe("browser session transport", () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
  });
  afterEach(() => vi.unstubAllGlobals());
  it("shares one refresh across simultaneous calls and keeps tokens out of browser storage", async () => {
    const fetcher = vi.fn(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
      return reply(session);
    });
    vi.stubGlobal("fetch", fetcher);
    const { refresh } = await import("./api");
    const [a, b] = await Promise.all([refresh(), refresh()]);
    expect(a.access_token).toBe(b.access_token);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });
  it("refreshes an expired request once and retries with a bearer token", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        reply({ code: "invalid_credentials", message: "Sign in" }, 401),
      )
      .mockResolvedValueOnce(reply(session))
      .mockResolvedValueOnce(reply({ private: "owner only" }));
    vi.stubGlobal("fetch", fetcher);
    const { api } = await import("./api");
    await expect(api("/users/me")).resolves.toEqual({ private: "owner only" });
    expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
      "/api/v1/users/me",
      "/api/v1/auth/refresh",
      "/api/v1/users/me",
    ]);
    expect(
      (fetcher.mock.calls[2][1].headers as Headers).get("Authorization"),
    ).toBe(`Bearer ${session.access_token}`);
  });
  it("clears the session when refresh is denied and does not loop", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          reply({ code: "invalid_credentials", message: "Sign in" }, 401),
        ),
    );
    const { api } = await import("./api");
    const listener = vi.fn();
    window.addEventListener("relations:logout", listener);
    await expect(api("/users/me")).rejects.toMatchObject({ status: 401 });
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener("relations:logout", listener);
  });
  it("clears an expired session when proactive refresh is denied before sending the private request", async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        reply({ code: "invalid_credentials", message: "Sign in" }, 401),
      )
      .mockResolvedValueOnce(reply({ public: true }));
    vi.stubGlobal("fetch", fetcher);
    const { acceptSession, api } = await import("./api");
    acceptSession({ ...session, expires_in: 0 } as Parameters<
      typeof acceptSession
    >[0]);
    const listener = vi.fn();
    window.addEventListener("relations:logout", listener);
    try {
      await expect(api("/users/me")).rejects.toMatchObject({ status: 401 });
      expect(fetcher.mock.calls.map((call) => call[0])).toEqual([
        "/api/v1/auth/refresh",
      ]);
      expect(listener).toHaveBeenCalledOnce();
      await api("/auth/config");
      expect(
        (fetcher.mock.calls[1][1].headers as Headers).has("Authorization"),
      ).toBe(false);
    } finally {
      window.removeEventListener("relations:logout", listener);
    }
  });
  it.each(["network", "server"])(
    "preserves the session after a temporary %s refresh failure and allows another attempt",
    async (failure) => {
      const fetcher = vi
        .fn()
        .mockResolvedValueOnce(
          reply({ code: "invalid_credentials", message: "Sign in" }, 401),
        );
      if (failure === "network")
        fetcher.mockRejectedValueOnce(new TypeError("Failed to fetch"));
      else
        fetcher.mockResolvedValueOnce(
          reply({ code: "internal_error", message: "Try again" }, 503),
        );
      fetcher
        .mockResolvedValueOnce(reply(session))
        .mockResolvedValueOnce(reply({ private: "owner only" }));
      vi.stubGlobal("fetch", fetcher);
      const { acceptSession, api, refresh } = await import("./api");
      acceptSession(session as Parameters<typeof acceptSession>[0]);
      const listener = vi.fn();
      window.addEventListener("relations:logout", listener);
      try {
        await expect(api("/users/me")).rejects.toMatchObject({
          status: failure === "network" ? 0 : 503,
        });
        expect(listener).not.toHaveBeenCalled();
        await refresh();
        await expect(api("/users/me")).resolves.toEqual({
          private: "owner only",
        });
        expect(fetcher).toHaveBeenCalledTimes(4);
      } finally {
        window.removeEventListener("relations:logout", listener);
      }
    },
  );
});

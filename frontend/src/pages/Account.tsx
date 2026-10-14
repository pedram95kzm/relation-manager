import { useState, type FormEvent } from "react";
import { api, clearSession, json, logout } from "../api";
import { useAuth } from "../auth";
import { usePreferences } from "../i18n";
import { Button, ErrorBox, PageTitle, useResource } from "../components/ui";
export function Account() {
  const { user, reload } = useAuth(),
    { t } = usePreferences(),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const config = useResource(() =>
    api<{ minimum_age: number }>("/auth/config", {}, false),
  );
  async function action(fn: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await fn();
      setNotice(t("saved"));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  function profile(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const values = Object.fromEntries(new FormData(e.currentTarget));
    void action(async () => {
      await api(
        "/users/me",
        json("PATCH", { ...values, age: Number(values.age) }),
      );
      await reload();
    });
  }
  function password(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const values = Object.fromEntries(new FormData(e.currentTarget));
    void action(async () => {
      await api("/users/me/password", json("POST", values));
      clearSession();
    });
  }
  function erase(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const values = Object.fromEntries(new FormData(e.currentTarget));
    if (window.confirm(t("deleteAccountHint")))
      void action(async () => {
        await api("/users/me", json("DELETE", values));
        clearSession();
      });
  }
  return (
    <>
      <PageTitle title={t("settings")} description={user?.email} />
      <ErrorBox message={error || config.error} />
      {notice && (
        <div className="notice" role="status">
          {notice}
        </div>
      )}
      <div className="account-grid">
        <section className="panel padded">
          <h2>{t("profile")}</h2>
          <form onSubmit={profile}>
            <label>
              {t("name")}
              <input
                name="name"
                defaultValue={user?.name}
                required
                maxLength={120}
              />
            </label>
            <div className="form-row">
              <label>
                {t("age")}
                <input
                  name="age"
                  type="number"
                  defaultValue={user?.age || ""}
                  min={config.data?.minimum_age || 18}
                  max={120}
                  required
                />
              </label>
              <label>
                {t("gender")}
                <select
                  name="gender"
                  defaultValue={user?.gender || ""}
                  required
                >
                  <option value="" disabled>
                    —
                  </option>
                  {[
                    "female",
                    "male",
                    "nonbinary",
                    "other",
                    "prefer_not_to_say",
                  ].map((g) => (
                    <option key={g} value={g}>
                      {t(g as "female")}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <Button disabled={busy}>{t("save")}</Button>
          </form>
        </section>
        <section className="panel padded">
          <h2>{t("security")}</h2>
          <form onSubmit={password}>
            <label>
              {t("currentPassword")}
              <input
                name="current_password"
                type="password"
                autoComplete="current-password"
                required
              />
            </label>
            <label>
              {t("newPassword")}
              <input
                name="new_password"
                type="password"
                autoComplete="new-password"
                required
                minLength={12}
              />
            </label>
            <small>{t("passwordHint")}</small>
            <Button disabled={busy}>{t("changePassword")}</Button>
          </form>
          <Button
            className="secondary"
            disabled={busy}
            onClick={() => action(() => logout(true))}
          >
            {t("logoutAll")}
          </Button>
        </section>
      </div>
      <section className="panel padded danger-zone">
        <h2>{t("deleteAccount")}</h2>
        <p>{t("deleteAccountHint")}</p>
        <form onSubmit={erase}>
          <label>
            {t("password")}
            <input
              type="password"
              name="password"
              autoComplete="current-password"
              required
            />
          </label>
          <Button className="danger" disabled={busy || user?.role === "admin"}>
            {t("deleteAccount")}
          </Button>
        </form>
      </section>
    </>
  );
}

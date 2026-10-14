import { useState, type FormEvent } from "react";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { Compass, ArrowRight, Sparkles } from "lucide-react";
import { usePreferences } from "../i18n";
import { useAuth } from "../auth";
import { api, login } from "../api";
import {
  Button,
  ErrorBox,
  Loading,
  PreferencesButtons,
  useResource,
} from "../components/ui";
export function AuthPage({ register = false }: { register?: boolean }) {
  const { t } = usePreferences(),
    { user, loading } = useAuth(),
    [params] = useSearchParams();
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const config = useResource(() =>
    api<{ google_enabled: boolean; minimum_age: number }>(
      "/auth/config",
      {},
      false,
    ),
  );
  if (loading) return <Loading />;
  if (user) return <Navigate to="/" replace />;
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const data = Object.fromEntries(new FormData(e.currentTarget));
    try {
      await login(register ? "/auth/register" : "/auth/login", data);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="auth-page">
      <section className="auth-story">
        <Link className="brand" to="/login">
          <Compass />
          relations.
        </Link>
        <div>
          <span className="eyebrow">
            <Sparkles size={16} />
            RELATIONS
          </span>
          <h1>{t("loginTitle")}</h1>
          <p>{t("loginText")}</p>
          <div className="auth-art" aria-hidden="true">
            <div />
            <div />
            <div />
          </div>
        </div>
        <small>{t("disclaimer")}</small>
      </section>
      <section className="auth-form">
        <PreferencesButtons />
        <div>
          <p className="eyebrow">{t("welcome")}</p>
          <h2>{register ? t("register") : t("login")}</h2>
          <ErrorBox
            message={
              error ||
              config.error ||
              (params.has("error")
                ? `${t("error")}: ${params.get("error")}`
                : "")
            }
          />
          <form onSubmit={submit}>
            {register && (
              <label>
                {t("name")}
                <input
                  name="name"
                  required
                  maxLength={120}
                  autoComplete="name"
                />
              </label>
            )}
            <label>
              {t("email")}
              <input
                name="email"
                type="email"
                required
                maxLength={254}
                autoComplete="email"
                dir="ltr"
              />
            </label>
            <label>
              {t("password")}
              <input
                name="password"
                type="password"
                required
                minLength={register ? 12 : undefined}
                autoComplete={register ? "new-password" : "current-password"}
              />
            </label>
            {register && <small>{t("passwordHint")}</small>}
            <Button type="submit" disabled={busy}>
              {register ? t("register") : t("login")}
              <ArrowRight size={17} />
            </Button>
          </form>
          {config.data?.google_enabled && (
            <a className="button secondary" href="/api/v1/auth/google">
              {t("google")}
            </a>
          )}
          <p className="auth-switch">
            <Link to={register ? "/login" : "/register"}>
              {register ? t("login") : t("register")}
            </Link>
          </p>
        </div>
      </section>
    </div>
  );
}

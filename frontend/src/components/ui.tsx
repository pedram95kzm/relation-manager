import { useEffect, useState, type ReactNode } from "react";
import { AlertCircle, LoaderCircle, Moon, Sun, Languages } from "lucide-react";
import { usePreferences } from "../i18n";
export function ErrorBox({ message }: { message: string }) {
  return message ? (
    <div className="notice error" role="alert">
      <AlertCircle size={18} />
      <span>{message}</span>
    </div>
  ) : null;
}
export function Loading() {
  const { t } = usePreferences();
  return (
    <div className="loading" role="status">
      <LoaderCircle className="spin" size={24} />
      {t("loading")}
    </div>
  );
}
export function PreferencesButtons() {
  const { locale, setLocale, theme, toggleTheme } = usePreferences();
  return (
    <div className="preferences">
      <button
        className="icon-button"
        onClick={() => setLocale(locale === "en" ? "fa" : "en")}
        aria-label={locale === "en" ? "Switch to Persian" : "Switch to English"}
      >
        <Languages size={18} />
        <span>{locale === "en" ? "FA" : "EN"}</span>
      </button>
      <button
        className="icon-button"
        onClick={toggleTheme}
        aria-label={theme === "light" ? "Use dark theme" : "Use light theme"}
      >
        {theme === "light" ? <Moon size={18} /> : <Sun size={18} />}
      </button>
    </div>
  );
}
export function Badge({ status }: { status: string }) {
  const { t } = usePreferences();
  const key = [
    "draft",
    "processing",
    "completed",
    "failed",
    "insufficient",
  ].includes(status)
    ? (status as "draft")
    : "draft";
  return <span className={`badge ${status}`}>{t(key)}</span>;
}
export function useResource<T>(
  fetcher: () => Promise<T>,
  dependencies: unknown[] = [],
) {
  const [data, setData] = useState<T | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [version, setVersion] = useState(0);
  useEffect(() => {
    let alive = true;
    setLoading(true);
    setData(null);
    setError("");
    fetcher()
      .then((v) => {
        if (alive) setData(v);
      })
      .catch((e) => {
        if (alive) setError(e.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [...dependencies, version]);
  return {
    data,
    setData,
    error,
    loading,
    reload: () => setVersion((v) => v + 1),
  };
}
export function PageTitle({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <header className="page-title">
      <div>
        {eyebrow && <p className="eyebrow">{eyebrow}</p>}
        <h1>{title}</h1>
        {description && <p>{description}</p>}
      </div>
      {actions}
    </header>
  );
}
export function Button({
  children,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button {...props} className={`button ${props.className || ""}`}>
      {children}
    </button>
  );
}

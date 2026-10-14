import { Link } from "react-router-dom";
import { useState } from "react";
import {
  ArrowUpRight,
  Fingerprint,
  HeartHandshake,
  MessagesSquare,
  Sparkles,
  Leaf,
  BookOpen,
  Trash2,
} from "lucide-react";
import { api } from "../api";
import { useAuth } from "../auth";
import { usePreferences } from "../i18n";
import {
  Badge,
  Button,
  ErrorBox,
  Loading,
  PageTitle,
  useResource,
} from "../components/ui";
import type { Analysis, Module, Page, Usage } from "../types";
export const modules: Module[] = ["self", "compatibility", "conflict"];
const icons = {
  self: Fingerprint,
  compatibility: HeartHandshake,
  conflict: MessagesSquare,
};
export function AnalysisList({
  items,
  onDeleted,
}: {
  items: Analysis[];
  onDeleted?: () => void;
}) {
  const { t, locale } = usePreferences(),
    [error, setError] = useState(""),
    [deleting, setDeleting] = useState("");
  async function erase(item: Analysis) {
    if (!window.confirm(t("deleteConfirm"))) return;
    setDeleting(item.id);
    setError("");
    try {
      await api(`/analyses/${item.id}`, { method: "DELETE" });
      onDeleted?.();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setDeleting("");
    }
  }
  if (!items.length)
    return (
      <div className="empty">
        <BookOpen size={30} />
        <h3>{t("empty")}</h3>
        <p>{t("emptyText")}</p>
      </div>
    );
  return (
    <div className="analysis-list">
      <ErrorBox message={error} />
      {items.map((item) => {
        const Icon = icons[item.module];
        return (
          <div key={item.id} className="analysis-row">
            <Link
              className="analysis-link"
              to={`/analyses/${item.id}${item.status === "draft" ? "/edit" : ""}`}
            >
              <span className={`module-icon ${item.module}`}>
                <Icon size={21} />
              </span>
              <div>
                <strong>{t(item.module)}</strong>
                <small>
                  {new Date(item.updated_at).toLocaleDateString(
                    locale === "en" ? "en-US" : "fa-IR",
                    { dateStyle: "medium" },
                  )}
                </small>
              </div>
              <Badge status={item.status} />
              <ArrowUpRight size={18} />
            </Link>
            {onDeleted && (
              <button
                className="icon-button danger"
                aria-label={`${t("delete")}: ${t(item.module)}`}
                disabled={!!deleting || item.status === "processing"}
                onClick={() => erase(item)}
              >
                <Trash2 size={15} />
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}
export function Dashboard() {
  const { user } = useAuth(),
    { t } = usePreferences();
  const history = useResource(() =>
    api<Page<Analysis>>("/analyses?page_size=5"),
  );
  const usage = useResource(() => api<Usage>("/analyses/usage"));
  return (
    <>
      <PageTitle
        eyebrow={`${t("dashboard")} / ${user?.name}`}
        title={t("welcome")}
        description={t("welcomeText")}
      />
      {!user?.age || !user.gender ? (
        <Link className="notice" to="/account">
          {t("profileRequired")}
          <ArrowUpRight size={17} />
        </Link>
      ) : null}
      <section className="hero-card">
        <div>
          <span className="pill">
            <Leaf size={14} />
            {t("privacy")}
          </span>
          <h2>{t("self")}</h2>
          <p>{t("selfText")}</p>
          <Link className="button" to="/reflect/self">
            {t("start")}
            <ArrowUpRight size={18} />
          </Link>
        </div>
        <div className="hero-art" aria-hidden="true">
          <div className="orbit orbit-one" />
          <div className="orbit orbit-two" />
          <div className="orbit orbit-three" />
          <span>
            <Fingerprint size={72} strokeWidth={1} />
          </span>
          <i className="spark spark-one">
            <Sparkles size={20} />
          </i>
          <i className="spark spark-two">
            <Leaf size={23} />
          </i>
        </div>
      </section>
      <section className="module-grid">
        {modules.map((module, i) => {
          const Icon = icons[module];
          return (
            <article className={`module-card ${module}`} key={module}>
              <div className="module-card-top">
                <span className={`module-icon ${module}`}>
                  <Icon size={25} />
                </span>
                <small>0{i + 1}</small>
              </div>
              <h3>{t(module)}</h3>
              <p>{t(`${module}Text`)}</p>
              <div className="module-card-bottom">
                <span>
                  {module === "self"
                    ? 36
                    : module === "compatibility"
                      ? "32 × 2"
                      : 15}{" "}
                  · {t("questions")}
                </span>
                <Link
                  to={`/reflect/${module}`}
                  aria-label={`${t("start")}: ${t(module)}`}
                >
                  <ArrowUpRight size={21} />
                </Link>
              </div>
            </article>
          );
        })}
      </section>
      <div className="dashboard-bottom">
        <section className="panel">
          <div className="panel-heading">
            <h2>{t("recent")}</h2>
            <Link to="/history">
              {t("viewAll")}
              <ArrowUpRight size={14} />
            </Link>
          </div>
          <ErrorBox message={history.error} />
          {history.loading ? (
            <Loading />
          ) : (
            <AnalysisList
              items={history.data?.items || []}
              onDeleted={history.reload}
            />
          )}
        </section>
        <section className="insight-card">
          <span className="module-icon self">
            <Leaf size={24} />
          </span>
          <h3>{t("privacy")}</h3>
          <p>{t("privacyText")}</p>
          <div className="quota">
            <small>{t("quota")}</small>
            <strong>
              {usage.data
                ? `${usage.data.user_used} / ${usage.data.user_limit}`
                : "—"}
            </strong>
            {usage.data && (
              <progress
                value={usage.data.user_used}
                max={usage.data.user_limit}
              />
            )}
          </div>
          <ErrorBox message={usage.error} />
        </section>
      </div>
    </>
  );
}
export function History() {
  const { t } = usePreferences(),
    [page, setPage] = useState(1),
    [module, setModule] = useState(""),
    [status, setStatus] = useState("");
  const history = useResource(
    () =>
      api<Page<Analysis>>(
        `/analyses?page=${page}&page_size=10${module ? `&module=${module}` : ""}${status ? `&status=${status}` : ""}`,
      ),
    [page, module, status],
  );
  return (
    <>
      <PageTitle title={t("history")} description={t("privacyText")} />
      <div className="toolbar">
        <label>
          {t("filter")}
          <select
            value={module}
            onChange={(e) => {
              setModule(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t("all")}</option>
            {modules.map((m) => (
              <option key={m} value={m}>
                {t(m)}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("results")}
          <select
            value={status}
            onChange={(e) => {
              setStatus(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t("all")}</option>
            {["draft", "processing", "completed", "insufficient", "failed"].map(
              (s) => (
                <option key={s} value={s}>
                  {t(s as "draft")}
                </option>
              ),
            )}
          </select>
        </label>
      </div>
      <ErrorBox message={history.error} />
      {history.loading ? (
        <Loading />
      ) : (
        <section className="panel">
          <AnalysisList
            items={history.data?.items || []}
            onDeleted={history.reload}
          />
        </section>
      )}
      <div className="pagination">
        <Button
          className="secondary"
          disabled={page === 1}
          onClick={() => setPage((p) => p - 1)}
        >
          {t("previous")}
        </Button>
        <span>
          {t("page")} {page}
        </span>
        <Button
          className="secondary"
          disabled={!history.data || page * 10 >= history.data.total}
          onClick={() => setPage((p) => p + 1)}
        >
          {t("next")}
        </Button>
      </div>
    </>
  );
}

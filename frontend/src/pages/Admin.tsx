import { useState, type FormEvent } from "react";
import { Navigate } from "react-router-dom";
import { Activity, CheckCircle2, Search } from "lucide-react";
import { api, json } from "../api";
import { useAuth } from "../auth";
import { usePreferences } from "../i18n";
import {
  Button,
  ErrorBox,
  Loading,
  PageTitle,
  useResource,
} from "../components/ui";
import type { Page, Provider, User, Module, Scores, Report } from "../types";
type Analytics = {
  users: number;
  active_users: number;
  disabled_users: number;
  analyses: number;
  generations: number;
  successful_generations: number;
  failed_generations: number;
  input_tokens: number;
  output_tokens: number;
  today_global_usage: number;
  daily: {
    day: string;
    generations: number;
    successes: number;
    failures: number;
  }[];
  modules: { module: string; count: number }[];
  providers: { provider: string; count: number }[];
};
type Audit = {
  id: string;
  action: string;
  actor_id?: string;
  target_user_id?: string;
  metadata: unknown;
  created_at: string;
};
export function Admin() {
  const { user } = useAuth(),
    { t } = usePreferences(),
    [tab, setTab] = useState("analytics");
  if (user?.role !== "admin") return <Navigate to="/" replace />;
  return (
    <>
      <PageTitle
        eyebrow="RELATIONS"
        title={t("admin")}
        description={t("privacyText")}
      />
      <div className="tabs" role="tablist">
        {["analytics", "users", "provider", "limits", "audit"].map((key) => (
          <button
            key={key}
            role="tab"
            aria-selected={tab === key}
            onClick={() => setTab(key)}
          >
            {t(key as "analytics")}
          </button>
        ))}
      </div>
      <div role="tabpanel">
        {tab === "analytics" ? (
          <AnalyticsPanel />
        ) : tab === "users" ? (
          <UsersPanel />
        ) : tab === "provider" ? (
          <ProviderPanel />
        ) : tab === "limits" ? (
          <LimitsPanel />
        ) : (
          <AuditPanel />
        )}
      </div>
    </>
  );
}
function AnalyticsPanel() {
  const { t, locale } = usePreferences(),
    resource = useResource(() => api<Analytics>("/admin/analytics?days=30"));
  if (resource.loading) return <Loading />;
  const data = resource.data;
  return (
    <>
      <ErrorBox message={resource.error} />
      {data && (
        <>
          <div className="stats-grid">
            {[
              [t("users"), data.users],
              [t("history"), data.analyses],
              [t("quota"), data.today_global_usage],
              [t("completed"), data.successful_generations],
            ].map(([label, value]) => (
              <div className="stat-card" key={label}>
                <small>{label}</small>
                <strong>{value}</strong>
              </div>
            ))}
          </div>
          <section className="panel padded">
            <h2>
              <Activity size={19} />
              {t("analytics")} · 30
            </h2>
            {data.daily?.length ? (
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>{t("page")}</th>
                      <th>{t("quota")}</th>
                      <th>{t("completed")}</th>
                      <th>{t("failed")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.daily.map((day) => (
                      <tr key={day.day}>
                        <td>{new Date(day.day).toLocaleDateString(locale)}</td>
                        <td>{day.generations}</td>
                        <td>{day.successes}</td>
                        <td>{day.failures}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p>{t("noData")}</p>
            )}
            <p>
              Tokens: {data.input_tokens} / {data.output_tokens}
            </p>
          </section>
        </>
      )}
    </>
  );
}
function UsersPanel() {
  const { t } = usePreferences(),
    [search, setSearch] = useState(""),
    [query, setQuery] = useState(""),
    [page, setPage] = useState(1),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const resource = useResource(
    () =>
      api<Page<User>>(
        `/admin/users?page=${page}&page_size=20&q=${encodeURIComponent(query)}`,
      ),
    [page, query],
  );
  const [erasing, setErasing] = useState<User | null>(null);
  async function erase(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!erasing) return;
    setBusy(true);
    setError("");
    const body = Object.fromEntries(new FormData(e.currentTarget));
    try {
      await api(`/admin/users/${erasing.id}`, json("DELETE", body));
      setErasing(null);
      resource.reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function update(user: User, body: unknown) {
    setBusy(true);
    setError("");
    try {
      await api(`/admin/users/${user.id}`, json("PATCH", body));
      resource.reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <form
        className="toolbar search-form"
        onSubmit={(e) => {
          e.preventDefault();
          setQuery(search);
          setPage(1);
        }}
      >
        <label className="search-label">
          <Search size={18} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("search")}
            aria-label={t("search")}
          />
        </label>
        <Button>{t("filter")}</Button>
      </form>
      <ErrorBox message={error || resource.error} />
      {erasing && (
        <section className="panel padded danger-zone">
          <h2>
            {t("delete")}: {erasing.email}
          </h2>
          <p>{t("eraseUser")}</p>
          <form onSubmit={erase}>
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
              {t("confirmEmail")}
              <input name="confirm_email" type="email" required dir="ltr" />
            </label>
            <div className="actions">
              <Button className="danger" disabled={busy}>
                {t("delete")}
              </Button>
              <Button
                className="secondary"
                type="button"
                onClick={() => setErasing(null)}
              >
                {t("cancel")}
              </Button>
            </div>
          </form>
        </section>
      )}
      {resource.loading ? (
        <Loading />
      ) : (
        <section className="panel table-scroll">
          <table>
            <thead>
              <tr>
                <th>{t("name")}</th>
                <th>{t("email")}</th>
                <th>{t("userLimit")}</th>
                <th>{t("settings")}</th>
              </tr>
            </thead>
            <tbody>
              {resource.data?.items.map((u) => (
                <tr key={u.id}>
                  <td>
                    {u.name}
                    <small> {u.role}</small>
                  </td>
                  <td dir="ltr">{u.email}</td>
                  <td>
                    <UserLimit
                      user={u}
                      disabled={busy || u.role === "admin"}
                      update={update}
                    />
                  </td>
                  <td>
                    <Button
                      className="secondary"
                      disabled={busy || u.role === "admin"}
                      onClick={() => update(u, { disabled: !u.disabled })}
                    >
                      {u.disabled ? t("enable") : t("disable")}
                    </Button>{" "}
                    <Button
                      className="secondary danger"
                      disabled={busy || u.role === "admin"}
                      onClick={() => setErasing(u)}
                    >
                      {t("delete")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      <div className="pagination">
        <Button
          disabled={page === 1}
          className="secondary"
          onClick={() => setPage((p) => p - 1)}
        >
          {t("previous")}
        </Button>
        <span>{page}</span>
        <Button
          disabled={!resource.data || page * 20 >= resource.data.total}
          className="secondary"
          onClick={() => setPage((p) => p + 1)}
        >
          {t("next")}
        </Button>
      </div>
    </>
  );
}
function UserLimit({
  user,
  disabled,
  update,
}: {
  user: User;
  disabled: boolean;
  update: (user: User, value: unknown) => Promise<void>;
}) {
  const { t } = usePreferences();
  return (
    <form
      className="inline-form"
      onSubmit={(e) => {
        e.preventDefault();
        const value = new FormData(e.currentTarget).get("limit");
        void update(user, { daily_limit: value ? Number(value) : null });
      }}
    >
      <input
        name="limit"
        type="number"
        min={1}
        max={10000}
        defaultValue={user.daily_limit || ""}
        disabled={disabled}
        aria-label={`${t("userLimit")}: ${user.name}`}
        placeholder="—"
      />
      <button
        className="icon-button"
        disabled={disabled}
        aria-label={t("save")}
      >
        <CheckCircle2 size={18} />
      </button>
    </form>
  );
}
function ProviderPanel() {
  const { t, locale } = usePreferences(),
    resource = useResource(() => api<Provider>("/admin/provider")),
    [error, setError] = useState(""),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState("");
  const [providerDirty, setProviderDirty] = useState(false);
  const [previewModule, setPreviewModule] = useState<Module>("self");
  const [preview, setPreview] = useState<{
    provider: string;
    external_request: boolean;
    scores: Scores;
    report: Report;
  } | null>(null);
  const [previewBusy, setPreviewBusy] = useState(false);
  const isMock = (selectedProvider || resource.data?.provider) === "mock";
  async function previewExample() {
    setPreviewBusy(true);
    setError("");
    setPreview(null);
    try {
      setPreview(
        await api(
          "/admin/provider/preview",
          json("POST", { module: previewModule, locale }),
        ),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setPreviewBusy(false);
    }
  }
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    const form = e.currentTarget;
    const values = Object.fromEntries(new FormData(form));
    const body = {
      ...values,
      max_tokens: Number(values.max_tokens),
      timeout_seconds: Number(values.timeout_seconds),
      retries: Number(values.retries),
      active: values.active === "on",
    };
    if (!values.api_key) delete (body as Record<string, unknown>).api_key;
    try {
      await api("/admin/provider", json("PUT", body));
      form.reset();
      setProviderDirty(false);
      resource.reload();
      setNotice(t("saved"));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function test() {
    setBusy(true);
    setError("");
    try {
      const result = await api<{ latency_ms: number }>(
        "/admin/provider/test",
        json("POST", {}),
      );
      setNotice(`${t("saved")} · ${result.latency_ms}ms`);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  if (resource.loading) return <Loading />;
  const provider = resource.data;
  return (
    <section className="panel padded">
      <h2>{t("provider")}</h2>
      <ErrorBox message={error || resource.error} />
      {notice && (
        <div className="notice" role="status">
          {notice}
        </div>
      )}
      {provider && (
        <form
          onSubmit={save}
          onChange={() => setProviderDirty(true)}
          key={`${provider.provider}-${provider.model}-${provider.has_key}`}
        >
          <div className="form-row">
            <label>
              {t("provider")}
              <select
                name="provider"
                value={selectedProvider || provider.provider || "openai"}
                onChange={(e) => setSelectedProvider(e.target.value)}
              >
                {["mock", "openai", "deepseek", "anthropic", "gemini"].map(
                  (p) => (
                    <option key={p} value={p}>
                      {p === "mock" ? t("localExample") : p}
                    </option>
                  ),
                )}
              </select>
            </label>
            <label>
              {t("model")}
              <input
                name="model"
                key={selectedProvider || provider.provider}
                defaultValue={
                  isMock
                    ? "local-example-v1"
                    : selectedProvider && selectedProvider !== provider.provider
                      ? ""
                      : provider.model
                }
                readOnly={isMock}
                required
                maxLength={120}
                dir="ltr"
              />
            </label>
          </div>
          {!isMock && (
            <label>
              {t("apiKey")}
              <input
                name="api_key"
                type="password"
                autoComplete="off"
                placeholder={provider.has_key ? "••••••••" : ""}
              />
            </label>
          )}
          {isMock && <div className="notice">{t("localExampleInfo")}</div>}
          <div className="form-row">
            <label>
              {t("maxTokens")}
              <input
                type="number"
                name="max_tokens"
                min={512}
                max={16000}
                defaultValue={provider.max_tokens || 4096}
                required
              />
            </label>
            <label>
              {t("timeoutSeconds")}
              <input
                type="number"
                name="timeout_seconds"
                min={1}
                max={120}
                defaultValue={provider.timeout_seconds || 60}
                required
              />
            </label>
            <label>
              {t("retries")}
              <input
                type="number"
                name="retries"
                min={0}
                max={3}
                defaultValue={provider.retries ?? 1}
                required
              />
            </label>
          </div>
          <label className="checkbox">
            <input
              name="active"
              type="checkbox"
              defaultChecked={provider.active}
            />
            <span>{t("active")}</span>
          </label>
          <div className="actions">
            <Button disabled={busy}>{t("save")}</Button>
            <Button
              className="secondary"
              type="button"
              disabled={busy || !provider.has_key || isMock || providerDirty}
              onClick={test}
            >
              {t("test")}
            </Button>
          </div>
        </form>
      )}
      <section className="example-preview">
        <h3>{t("previewExample")}</h3>
        <p className="muted">{t("localExampleInfo")}</p>
        <div className="form-row">
          <label>
            {t("questions")}
            <select
              value={previewModule}
              onChange={(e) => setPreviewModule(e.target.value as Module)}
            >
              {(["self", "compatibility", "conflict"] as Module[]).map((m) => (
                <option key={m} value={m}>
                  {t(m)}
                </option>
              ))}
            </select>
          </label>
        </div>
        <Button
          className="secondary"
          type="button"
          disabled={previewBusy}
          onClick={previewExample}
        >
          {t("previewExample")}
        </Button>
        {preview && (
          <div className="preview-result">
            <h3>{preview.report.title}</h3>
            <p>{preview.report.summary}</p>
            <details>
              <summary>{t("responseJson")}</summary>
              <pre className="json-preview" dir="ltr">
                {JSON.stringify(preview, null, 2)}
              </pre>
            </details>
          </div>
        )}
      </section>
    </section>
  );
}
function LimitsPanel() {
  const { t } = usePreferences(),
    resource = useResource(() =>
      api<{ global_daily: number; user_daily: number }>("/admin/limits"),
    ),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [saved, setSaved] = useState(false);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setSaved(false);
    setError("");
    const values = Object.fromEntries(new FormData(e.currentTarget));
    try {
      await api(
        "/admin/limits",
        json("PUT", {
          global_daily: Number(values.global_daily),
          user_daily: Number(values.user_daily),
        }),
      );
      resource.reload();
      setSaved(true);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel padded">
      <h2>{t("limits")}</h2>
      <ErrorBox message={error || resource.error} />
      {saved && (
        <div className="notice" role="status">
          {t("saved")}
        </div>
      )}
      {resource.loading ? (
        <Loading />
      ) : (
        resource.data && (
          <form onSubmit={save}>
            <div className="form-row">
              <label>
                {t("globalLimit")}
                <input
                  type="number"
                  name="global_daily"
                  defaultValue={resource.data.global_daily}
                  min={1}
                  max={1000000}
                  required
                />
              </label>
              <label>
                {t("userLimit")}
                <input
                  type="number"
                  name="user_daily"
                  defaultValue={resource.data.user_daily}
                  min={1}
                  max={10000}
                  required
                />
              </label>
            </div>
            <Button disabled={busy}>{t("save")}</Button>
          </form>
        )
      )}
    </section>
  );
}
function AuditPanel() {
  const { t, locale } = usePreferences(),
    [page, setPage] = useState(1),
    resource = useResource(
      () => api<Page<Audit>>(`/admin/audit?page=${page}&page_size=20`),
      [page],
    );
  return (
    <>
      <ErrorBox message={resource.error} />
      {resource.loading ? (
        <Loading />
      ) : (
        <section className="panel table-scroll">
          <table>
            <thead>
              <tr>
                <th>{t("page")}</th>
                <th>{t("audit")}</th>
                <th>{t("users")}</th>
              </tr>
            </thead>
            <tbody>
              {resource.data?.items.map((event) => (
                <tr key={event.id}>
                  <td>{new Date(event.created_at).toLocaleString(locale)}</td>
                  <td>{event.action}</td>
                  <td>
                    <code>
                      {event.target_user_id?.slice(0, 8) ||
                        event.actor_id?.slice(0, 8) ||
                        "—"}
                    </code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {!resource.data?.items.length && (
            <p className="padded">{t("noData")}</p>
          )}
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
        <span>{page}</span>
        <Button
          className="secondary"
          disabled={!resource.data || page * 20 >= resource.data.total}
          onClick={() => setPage((p) => p + 1)}
        >
          {t("next")}
        </Button>
      </div>
    </>
  );
}

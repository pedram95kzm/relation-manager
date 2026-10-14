import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Download, Trash2, ShieldCheck } from "lucide-react";
import { api, json } from "../api";
import { usePreferences } from "../i18n";
import {
  Badge,
  Button,
  ErrorBox,
  Loading,
  PageTitle,
  useResource,
} from "../components/ui";
import type { Analysis, Scores } from "../types";
export function ScoreTables({ scores }: { scores: Scores }) {
  const { t, locale } = usePreferences();
  return (
    <>
      {Object.entries(scores.respondents || {}).map(([r, value]) => (
        <section className="panel padded score-panel" key={r}>
          <h2>{r === "self" ? t("selfRespondent") : t("partnerRespondent")}</h2>
          <p>{t("scoreHint")}</p>
          <div className="table-scroll">
            <table>
              <caption className="sr-only">{t("scores")}</caption>
              <thead>
                <tr>
                  <th>{t("dimension")}</th>
                  <th>{t("score")}</th>
                  <th>{t("completeness")}</th>
                  <th>{t("unknown")}</th>
                </tr>
              </thead>
              <tbody>
                {value.dimensions.map((d) => (
                  <tr key={d.id}>
                    <th scope="row">{d.label[locale]}</th>
                    <td>
                      <div className="score-cell">
                        <span className="bar-track" aria-hidden="true">
                          {d.score !== null && (
                            <span style={{ width: `${d.score}%` }} />
                          )}
                        </span>
                        <strong>
                          {d.score === null ? "—" : d.score.toFixed(1)}
                        </strong>
                      </div>
                    </td>
                    <td>
                      {d.answered}/{d.total} ·{" "}
                      {Math.round(d.completeness * 100)}%
                    </td>
                    <td>{d.unknown}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ))}
    </>
  );
}
const references: Record<string, string> = {
  bfi2: "https://doi.org/10.1037/pspp0000096",
  attachment: "https://doi.org/10.1037/0022-3514.78.2.350",
  emotion_regulation: "https://doi.org/10.1037/0022-3514.85.2.348",
  emotion_awareness: "https://doi.org/10.1023/B:JOBA.0000007455.08539.94",
  communication: "https://doi.org/10.1037/0022-3514.59.1.73",
  conflict_styles: "https://doi.org/10.1177/001316447703700204",
  relationship_similarity: "https://doi.org/10.1037/a0020385",
};
export function Results() {
  const { id } = useParams(),
    navigate = useNavigate(),
    { t } = usePreferences(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const resource = useResource(() => api<Analysis>(`/analyses/${id}`), [id]);
  useEffect(() => {
    if (resource.data?.status !== "processing") return;
    const timer = window.setTimeout(resource.reload, 3000);
    return () => window.clearTimeout(timer);
  }, [resource.data]);
  if (resource.loading && !resource.data) return <Loading />;
  const analysis = resource.data;
  if (!analysis)
    return (
      <>
        <ErrorBox message={resource.error} />
        <Button onClick={resource.reload}>{t("retry")}</Button>
      </>
    );
  if (analysis.status === "draft")
    return (
      <Link className="button" to={`/analyses/${id}/edit`}>
        {t("next")}
      </Link>
    );
  async function erase() {
    if (!window.confirm(t("deleteConfirm"))) return;
    setBusy(true);
    try {
      await api(`/analyses/${id}`, { method: "DELETE" });
      navigate("/history");
    } catch (e) {
      setError((e as Error).message);
      setBusy(false);
    }
  }
  async function deterministic() {
    setBusy(true);
    try {
      await api(
        `/analyses/${id}/submit`,
        json("POST", {
          revision: analysis!.revision,
          consent: true,
          generate_report: false,
        }),
      );
      resource.reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const report = analysis.report,
    similarity = analysis.scores?.compatibility;
  return (
    <>
      <PageTitle
        eyebrow={t(analysis.module)}
        title={report?.title || t("results")}
        description={`${analysis.questionnaire_version} · ${analysis.scoring_version}`}
        actions={
          <div className="actions">
            <Button className="secondary" onClick={() => window.print()}>
              <Download size={16} />
              {t("export")}
            </Button>
            <Button
              className="icon-button danger"
              disabled={busy || analysis.status === "processing"}
              onClick={erase}
              aria-label={t("delete")}
            >
              <Trash2 size={17} />
            </Button>
          </div>
        }
      />
      <Badge status={analysis.status} />
      {report?.kind === "example" && (
        <div className="notice">{t("localExampleInfo")}</div>
      )}
      <ErrorBox message={error || resource.error} />
      {["failed", "insufficient"].includes(analysis.status) && (
        <div className="notice">
          <span>{analysis.failure_code || t("insufficient")}</span>
          <Link className="button secondary" to={`/analyses/${id}/edit`}>
            {t("review")}
          </Link>
          {analysis.status === "failed" && (
            <Button
              className="secondary"
              disabled={busy}
              onClick={deterministic}
            >
              {t("scores")}
            </Button>
          )}
        </div>
      )}
      {analysis.status === "processing" ? (
        <Loading />
      ) : (
        <>
          {report && (
            <section className="result-summary">
              <ShieldCheck size={25} />
              <div>
                <p className="eyebrow">
                  {report.kind === "example"
                    ? t("localExampleReport")
                    : report.kind === "ai"
                      ? `${analysis.provider} · ${analysis.model}`
                      : t("scores")}
                </p>
                <p>{report.summary}</p>
              </div>
            </section>
          )}
          {similarity && (
            <section className="panel padded similarity">
              <div>
                <h2>{t("similarity")}</h2>
                <p>{t("similarityHint")}</p>
                <small>
                  {t("completeness")}: {Math.round(similarity.coverage * 100)}%
                  · {similarity.included_dimensions?.length || 0}/15
                </small>
                <p className="muted">{similarity.status}</p>
              </div>
              <strong>
                {similarity.index === null ? "—" : similarity.index.toFixed(1)}
                <small>/100</small>
              </strong>
            </section>
          )}
          {analysis.scores && <ScoreTables scores={analysis.scores} />}
          <div className="report-sections">
            {report?.sections?.map((s) => (
              <section className="panel padded" key={s.id}>
                <h2>{s.title}</h2>
                <p className="preserve-lines">{s.body}</p>
              </section>
            ))}
          </div>
          {report?.recommendations?.length ? (
            <section className="panel padded">
              <h2>{t("next")}</h2>
              <ol>
                {report.recommendations.map((r, i) => (
                  <li key={i}>{r}</li>
                ))}
              </ol>
            </section>
          ) : null}
          {report?.safety && (
            <section
              className={`notice safety ${report.safety.level === "caution" ? "caution" : ""}`}
            >
              <div>
                <h2>{report.safety.message}</h2>
                <ul>
                  {report.safety.support.map((s, i) => (
                    <li key={i}>{s}</li>
                  ))}
                </ul>
              </div>
            </section>
          )}
          <section className="limitations">
            <h3>{t("limitations")}</h3>
            <p>{t("disclaimer")}</p>
            <ul>
              {report?.limitations.map((l, i) => (
                <li key={i}>{l}</li>
              ))}
            </ul>
            {report?.references?.map(
              (r) =>
                references[r] && (
                  <a
                    key={r}
                    href={references[r]}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {r} ↗{" "}
                  </a>
                ),
            )}
          </section>
        </>
      )}
    </>
  );
}

import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams, useBlocker } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  Cloud,
  LoaderCircle,
} from "lucide-react";
import { api, ApiError, json } from "../api";
import { usePreferences } from "../i18n";
import { useAuth } from "../auth";
import {
  Button,
  ErrorBox,
  Loading,
  PageTitle,
  useResource,
} from "../components/ui";
import type {
  Analysis,
  AnalysisContext,
  Answers,
  Definition,
  Usage,
} from "../types";

export function answeredCount(answers: Answers) {
  return Object.values(answers).reduce(
    (n, values) => n + Object.keys(values).length,
    0,
  );
}
export function Wizard() {
  const params = useParams(),
    navigate = useNavigate(),
    { t, locale } = usePreferences(),
    { user } = useAuth();
  const [draft, setDraft] = useState<Analysis | null>(null),
    [definition, setDefinition] = useState<Definition | null>(null);
  const [answers, setAnswers] = useState<Answers>({ self: {} }),
    [context, setContext] = useState<AnalysisContext>({});
  const [step, setStep] = useState(0),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [dirty, setDirty] = useState(false),
    [saved, setSaved] = useState(false);
  const [consent, setConsent] = useState(false),
    [aiConsent, setAIConsent] = useState(false);
  const saving = useRef(false),
    revision = useRef<Analysis | null>(null),
    changes = useRef(0),
    submitted = useRef(false);
  const leaving = useRef(false);
  const [autosaveStopped, setAutosaveStopped] = useState(false);
  const blocker = useBlocker(
    () => dirty && !submitted.current && !leaving.current,
  );
  useEffect(() => {
    if (blocker.state === "blocked") {
      if (window.confirm(t("unsaved"))) blocker.proceed();
      else blocker.reset();
    }
  }, [blocker, t]);
  const usage = useResource(() => api<Usage>("/analyses/usage"));
  // Drafts are created only by an explicit save, never by a mount effect.
  useEffect(() => {
    let alive = true;
    async function load() {
      let existing: Analysis | null = null;
      if (params.id) existing = await api<Analysis>(`/analyses/${params.id}`);
      if (
        existing &&
        !["draft", "failed", "insufficient"].includes(existing.status)
      ) {
        navigate(`/analyses/${existing.id}`, { replace: true });
        return;
      }
      const bank = await api<Definition>(
        `/questionnaires/${existing?.module || params.module}?version=${existing?.questionnaire_version || "1.0.0"}`,
        {},
        false,
      );
      if (!alive) return;
      setDefinition(bank);
      setDraft(existing);
      revision.current = existing;
      setAnswers(
        existing?.answers ||
          (bank.module === "compatibility"
            ? { self: {}, partner: {} }
            : { self: {} }),
      );
      setContext(existing?.context || {});
      if (existing) {
        const pages = Math.ceil(bank.questions.length / 6);
        const people =
          bank.module === "compatibility" ? ["self", "partner"] : ["self"];
        let resume = pages * people.length;
        for (let r = people.length - 1; r >= 0; r--) {
          const missing = bank.questions.findIndex(
            (q) => !(q.id in (existing!.answers[people[r]] || {})),
          );
          if (missing >= 0) resume = r * pages + Math.floor(missing / 6);
        }
        if (
          resume === pages * people.length &&
          bank.context_fields.length &&
          bank.context_fields.every(
            (f) =>
              !f.required ||
              (existing!.context[f.id] !== undefined &&
                existing!.context[f.id] !== ""),
          )
        )
          resume++;
        setStep(resume);
      }
      setLoading(false);
    }
    load().catch((e) => {
      if (alive) {
        setError(e.message);
        setLoading(false);
      }
    });
    return () => {
      alive = false;
    };
  }, [params.id, params.module, navigate]);
  useEffect(() => {
    const beforeUnload = (e: BeforeUnloadEvent) => {
      if (dirty) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => window.removeEventListener("beforeunload", beforeUnload);
  }, [dirty]);
  useEffect(() => {
    if (!dirty || busy || !definition || autosaveStopped) return;
    const timer = window.setTimeout(() => {
      void saveDraft().catch((e) => {
        setError(e.message);
        setAutosaveStopped(true);
      });
    }, 1500);
    return () => window.clearTimeout(timer);
  }, [answers, context, dirty, busy, definition, autosaveStopped]);
  function changed() {
    changes.current++;
    setDirty(true);
    setSaved(false);
    setError("");
  }
  async function saveDraft() {
    if (saving.current) throw new Error(t("loading"));
    if (!definition) throw new Error(t("loading"));
    saving.current = true;
    setBusy(true);
    const snapshot = changes.current;
    try {
      const current = revision.current;
      const value = current
        ? await api<Analysis>(
            `/analyses/${current.id}`,
            json("PATCH", {
              revision: current.revision,
              answers,
              context,
              locale,
            }),
          )
        : await api<Analysis>(
            "/analyses",
            json("POST", {
              module: definition.module,
              locale,
              questionnaire_version: definition.version,
              answers,
              context,
            }),
          );
      revision.current = value;
      setDraft(value);
      setAutosaveStopped(false);
      if (snapshot === changes.current) {
        setDirty(false);
        setSaved(true);
      }
      return value;
    } finally {
      saving.current = false;
      setBusy(false);
    }
  }
  if (loading) return <Loading />;
  if (!definition)
    return (
      <>
        <ErrorBox message={error} />
        <Link to="/">{t("home")}</Link>
      </>
    );
  if (!user?.age || !user.gender)
    return (
      <Link className="notice" to="/account">
        {t("profileRequired")}
      </Link>
    );
  const bank = definition,
    respondents =
      bank.module === "compatibility" ? ["self", "partner"] : ["self"],
    perRespondent = Math.ceil(bank.questions.length / 6);
  const questionSteps = perRespondent * respondents.length,
    hasContext = bank.context_fields.length > 0,
    reviewStep = questionSteps + (hasContext ? 1 : 0);
  const reviewing = step === reviewStep,
    contextStep = hasContext && step === questionSteps;
  const respondent = respondents[Math.floor(step / perRespondent)] || "self";
  const items = bank.questions.slice(
    (step % perRespondent) * 6,
    ((step % perRespondent) + 1) * 6,
  );
  const count = answeredCount(answers),
    total = bank.questions.length * respondents.length;
  const missing = respondents.reduce(
    (n, r) =>
      n +
      bank.questions.filter((q) => q.required && !(q.id in (answers[r] || {})))
        .length,
    0,
  );
  const contextMissing = bank.context_fields.some(
    (f) => f.required && (context[f.id] === undefined || context[f.id] === ""),
  );
  const canContinue = contextStep
    ? !contextMissing
    : items.every((q) => !q.required || q.id in (answers[respondent] || {}));
  async function next() {
    setError("");
    try {
      await saveDraft();
      setStep((s) => s + 1);
      window.scrollTo({ top: 0, behavior: "smooth" });
    } catch (e) {
      setError((e as Error).message);
    }
  }
  async function submit() {
    setError("");
    try {
      const value = await saveDraft();
      setBusy(true);
      const result = await api<Analysis>(
        `/analyses/${value.id}/submit`,
        json("POST", {
          revision: value.revision,
          consent,
          generate_report: aiConsent,
          ai_consent: aiConsent && !usage.data?.is_example,
          local_example: aiConsent && usage.data?.is_example === true,
        }),
      );
      submitted.current = true;
      setDirty(false);
      navigate(`/analyses/${result.id}`, { replace: true });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  function choose(id: string, value: number | null) {
    setAnswers((a) => ({
      ...a,
      [respondent]: { ...a[respondent], [id]: value },
    }));
    changed();
  }
  return (
    <>
      <PageTitle
        eyebrow={t(bank.module)}
        title={bank.title[locale]}
        description={bank.description[locale]}
        actions={
          <Button
            className="secondary"
            disabled={busy}
            onClick={() => {
              void saveDraft()
                .then(() => {
                  leaving.current = true;
                  navigate("/history");
                })
                .catch((e) => setError(e.message));
            }}
          >
            {t("saveExit")}
          </Button>
        }
      />
      <section className="wizard-progress">
        <div>
          <span>
            {reviewing
              ? t("review")
              : contextStep
                ? t("context")
                : respondent === "self"
                  ? t("selfRespondent")
                  : t("partnerRespondent")}
          </span>
          <small aria-live="polite">
            {saved ? (
              <>
                <Cloud size={14} />
                {t("saved")}
              </>
            ) : busy ? (
              <LoaderCircle size={14} className="spin" />
            ) : (
              `${count} / ${total}`
            )}
          </small>
        </div>
        <progress max={reviewStep} value={step} />
        <span className="step-label">
          {t("page")} {step + 1} / {reviewStep + 1}
        </span>
      </section>
      <ErrorBox message={error || usage.error} />
      {reviewing ? (
        <section className="panel padded">
          <h2>{t("review")}</h2>
          <p>{t("reviewHint")}</p>
          <div className="review-counts">
            <span>
              {t("answered")}: {count}
            </span>
            <span>
              {t("omitted")}: {missing}
            </span>
          </div>
          {respondents.map((r, ri) => (
            <div key={r}>
              <h3>
                {r === "self" ? t("selfRespondent") : t("partnerRespondent")}
              </h3>
              <div className="review-list">
                {bank.questions.map((q, i) => (
                  <button
                    key={q.id}
                    onClick={() =>
                      setStep(ri * perRespondent + Math.floor(i / 6))
                    }
                  >
                    <span>{q.text[locale]}</span>
                    <strong>
                      {answers[r]?.[q.id] === null
                        ? bank.unknown_label[locale]
                        : (answers[r]?.[q.id] ?? "—")}
                    </strong>
                  </button>
                ))}
              </div>
            </div>
          ))}
          {hasContext && (
            <div className="review-list">
              {bank.context_fields.map((f) => (
                <button key={f.id} onClick={() => setStep(questionSteps)}>
                  <span>{f.text[locale]}</span>
                  <strong>{String(context[f.id] ?? "—")}</strong>
                </button>
              ))}
            </div>
          )}
          <div className="consent-box">
            <label className="checkbox">
              <input
                type="checkbox"
                checked={consent}
                onChange={(e) => setConsent(e.target.checked)}
              />
              <span>{t("consent")}</span>
            </label>
            {usage.data?.report_available && (
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={aiConsent}
                  onChange={(e) => setAIConsent(e.target.checked)}
                />
                <span>
                  {t(
                    usage.data?.is_example ? "localExampleChoice" : "aiConsent",
                  )}
                </span>
              </label>
            )}
            <p>{t("disclaimer")}</p>
          </div>
        </section>
      ) : contextStep ? (
        <section className="panel padded">
          <h2>{t("context")}</h2>
          {bank.context_fields.map((field) => (
            <label
              key={field.id}
              className={field.type === "boolean" ? "checkbox" : ""}
            >
              {field.type === "boolean" ? (
                <>
                  <input
                    type="checkbox"
                    checked={context[field.id] === true}
                    onChange={(e) => {
                      setContext((c) => ({
                        ...c,
                        [field.id]: e.target.checked,
                      }));
                      changed();
                    }}
                  />
                  <span>{field.text[locale]}</span>
                </>
              ) : (
                <>
                  {field.text[locale]}
                  {field.required && " *"}
                  {field.type === "enum" ? (
                    <select
                      required={field.required}
                      value={String(context[field.id] || "")}
                      onChange={(e) => {
                        setContext((c) => ({
                          ...c,
                          [field.id]: e.target.value,
                        }));
                        changed();
                      }}
                    >
                      <option value="" disabled>
                        —
                      </option>
                      {field.options?.map((o) => (
                        <option value={o.value} key={o.value}>
                          {o.label[locale]}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <textarea
                      rows={3}
                      value={String(context[field.id] || "")}
                      maxLength={field.max_length || 2000}
                      required={field.required}
                      onChange={(e) => {
                        setContext((c) => ({
                          ...c,
                          [field.id]: e.target.value,
                        }));
                        changed();
                      }}
                    />
                  )}
                </>
              )}
            </label>
          ))}
        </section>
      ) : (
        <section className="questions">
          <p className="question-hint">{t("answerHint")}</p>
          {items.map((question, i) => (
            <fieldset key={question.id} className="question-card">
              <legend>
                <span className="question-number">
                  {(step % perRespondent) * 6 + i + 1}
                </span>
                {question.text[locale]}
              </legend>
              <div className="likert">
                {question.options.map((option) => (
                  <label
                    key={option.value}
                    className={
                      answers[respondent]?.[question.id] === option.value
                        ? "selected"
                        : ""
                    }
                  >
                    <input
                      type="radio"
                      name={`${respondent}-${question.id}`}
                      value={option.value}
                      checked={
                        answers[respondent]?.[question.id] === option.value
                      }
                      onChange={() => choose(question.id, option.value)}
                    />
                    <strong>{option.value}</strong>
                    <span>{option.label[locale]}</span>
                  </label>
                ))}
              </div>
              <label className="unknown-choice">
                <input
                  type="radio"
                  name={`${respondent}-${question.id}`}
                  checked={
                    question.id in (answers[respondent] || {}) &&
                    answers[respondent][question.id] === null
                  }
                  onChange={() => choose(question.id, null)}
                />
                {bank.unknown_label[locale]}
              </label>
            </fieldset>
          ))}
        </section>
      )}
      <div className="wizard-actions">
        <Button
          className="secondary"
          disabled={busy || step === 0}
          onClick={() => setStep((s) => s - 1)}
        >
          <ArrowLeft size={17} />
          {t("back")}
        </Button>
        {reviewing ? (
          <Button
            disabled={busy || !consent || missing > 0 || contextMissing}
            onClick={submit}
          >
            {busy ? t("submitting") : t("submit")}
            <CheckCircle2 size={18} />
          </Button>
        ) : (
          <Button disabled={busy || !canContinue} onClick={next}>
            {step + 1 === reviewStep ? t("review") : t("next")}
            <ArrowRight size={17} />
          </Button>
        )}
      </div>
      {draft && (
        <small className="draft-id">
          {t("draft")} · {draft.id.slice(0, 8)}
        </small>
      )}
    </>
  );
}

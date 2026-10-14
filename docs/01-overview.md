# Project overview

Reviewed **2026-10-14** against the application source. Execution evidence belongs in [Development](04-development.md#verification-results); setup belongs in the [User guide](02-user-guide.md).

## Purpose and audience

Relations helps adults reflect on personality-related tendencies, compare consenting respondents' answers, and consider interpersonal conflict. The application provides structured original questionnaires, transparent arithmetic, private history, and optional AI narratives. It is an educational tool, not a clinical assessment, treatment, emergency service, or relationship forecast.

## Delivered features

| Area | Current behavior | Evidence |
| --- | --- | --- |
| React interface | Responsive dashboard/navigation, English/Persian with Persian default and RTL/LTR, light/dark preferences | [frontend](../frontend/src/main.tsx) |
| Accounts | Register/login, memory-only access JWT, HttpOnly refresh cookie, coordinated refresh, Google callback page, profile, password change, logout/all, password-account erasure | [auth client](../frontend/src/api.ts), [backend auth](../backend/internal/auth/service.go) |
| Reflection wizards | Self, compatibility, conflict; explicit unknown choices, context, autosave, resume, review, educational consent, separate AI sharing consent | [wizard](../frontend/src/pages/Wizard.tsx) |
| Saved analyses | Owner-scoped draft CRUD, revision checks, final validation, deterministic scores, submission, paginated/filterable history, reports, deletion | [analysis service](../backend/internal/analysis/service.go) |
| Results | Separate score bars/text tables, unknown gaps, numerical completeness, compatibility index with coverage, limitations, narrative sections, print/PDF | [results](../frontend/src/pages/Results.tsx) |
| AI | Four live adapters, local example mode/no-network preview, six prompts, strict report/reference/score checks, encrypted live credentials, timeout/retry handling | [AI](../backend/internal/ai/service.go) |
| Quotas and recovery | Atomic user/global UTC generation attempt limits, per-user overrides, sanitized usage events, interrupted processing recovery | [quota service](../backend/internal/analysis/quota.go) |
| Administration | Real aggregate activity, paginated user search, disable/enable, per-user limits, ordinary-account erasure with administrator password, provider configuration/test, global limits, audit | [admin](../backend/internal/admin/service.go), [admin UI](../frontend/src/pages/Admin.tsx) |
| Operation | Immutable SQL migration/catalog seeding, live/ready endpoints, full-stack Compose proxy, setup/verification/backup scripts, CI | [server](../backend/cmd/server/main.go), [Compose](../docker-compose.yml), [scripts](../scripts) |

Development/test creates a default administrator and ordinary user when their role is absent, with public credentials documented in the User guide; production automatic defaults are disabled. Existing accounts are never changed. Local example reports and previews never send prompts externally.

Google-only users cannot erase themselves through the password workflow. A password-authenticated administrator can erase an ordinary Google-only account through the management API. Administrator targets are protected. Google matching-email linking is intentionally unavailable.

## Main user journeys

1. Run the stack and create an account. Complete name, eligible age, and gender before creating/submitting reflections.
2. Choose a module. Answer each required question numerically or explicitly unknown. Drafts save after a short pause and when continuing/exiting. Resume starts at the first unanswered page.
3. For compatibility, record both respondents' responses and the asserted origin/sharing consent. For conflict, provide relationship type, reported safety, and a topic.
4. Review responses and consent to storage/processing. AI sharing is a separate optional checkbox, shown when an active provider is available. Without AI, deterministic results remain usable.
5. View scores and limitations, filter saved history, delete records, or print a report. Failed/insufficient records can be reviewed and resubmitted; completed records are immutable.
6. A development default or explicitly seeded administrator signs in through ordinary login and manages operational metadata, local examples/live providers, limits, and accounts.

## Scientific interpretation

| Module | Bank | Interpretation |
| --- | --- | --- |
| Self | 36 items, 10 dimensions | Personality-related, attachment-related, emotion, and communication tendencies |
| Compatibility | 32 items per respondent, 15 dimensions, 2 context fields | Descriptive similarity over sufficient shared dimensions; requires asserted independent responses and partner sharing consent |
| Conflict | 15 items, 5 dimensions, 8 context fields | Response styles and supplied situational context; context never becomes invented numerical trait evidence |

Items and translations are original and construct-inspired. They are not licensed reproductions or validated versions of the cited measures. Reliability, clinical validity, population norms, and cross-language equivalence remain unknown.

Numeric sufficiency is `max(2, ceil(2*n/3))` per dimension. Reverse keys use `6-x`; scores are `25*(keyed mean-1)`. Explicit unknown and omission remain distinct; insufficient scores are null. Completeness is a fraction of numeric responses, not statistical confidence. Self availability needs at least three sufficient dimensions; valid conflict context can produce an available context-led reflection with null style scores.

Compatibility averages `100-abs(self_score-partner_score)` across at least three sufficient shared dimensions. Coverage and omitted dimensions remain visible. Consent and independence are respondent assertions, not verified partner identities. The index is neither a probability nor a forecast. See [scoring contracts and references](04-development.md#questionnaire-and-scoring-contracts).

## Technology

The backend is a modular Go/Gin/GORM service with PostgreSQL and embedded SQL/JSON/prompts. The frontend uses React, TypeScript, Vite, React Router, Lucide icons, and locally served Manrope/Vazirmatn fonts. Exact dependencies are pinned in [go.mod](../backend/go.mod), [go.sum](../backend/go.sum), [package.json](../frontend/package.json), and [package-lock.json](../frontend/package-lock.json).

Native Windows Go 1.27.1, Node 24.15.0/npm 10.9.2, PostgreSQL 15.5, and Chrome were used for development checks. Compose declares PostgreSQL 17.11 and non-root backend/frontend containers. Availability of image tags was checked; container runtime was not available locally.

## Limits and unresolved choices

- Live Google and provider/model availability require real credentials, account access, and outbound connectivity. Offline protocol tests do not verify paid provider accounts.
- Production TLS ingress, infrastructure, release ownership, legal privacy/retention policy, and incident response remain operator decisions. The source uses the [MIT License](../LICENSE). No deployment was published.
- Password reset/email verification, explicit identity linking, verified partner invitations, and independent partner accounts remain optional extensions outside the delivered reflection workflows.
- Questionnaires/scoring currently support version `1.0.0`. Stored versions are explicit, but multi-version dispatch must be implemented before introducing another version.
- AI schema checks cannot establish clinical truth or fully enforce the meaning of generated prose. Rendered scores always come from deterministic calculations.
- Generation limits count admitted attempts, not money or individual retry HTTP calls. Provider connection tests have separate throttling and may incur provider cost.
- Answers, narratives, reports, and database backups are sensitive plaintext unless storage is encrypted externally. Only provider keys receive application encryption.
- Account/history retention is not automatically scheduled. Users can delete their records; administrators can erase ordinary accounts. Operational events retain sanitized data.

## Glossary

| Term | Meaning |
| --- | --- |
| Unknown | Explicit null response, excluded from arithmetic |
| Omitted | Question key absent from the answer map |
| Revision | Positive draft/lifecycle version used to reject stale writes |
| Numeric completeness | Numeric responses divided by eligible items |
| Similarity | Transparent answer-range arithmetic over sufficient shared dimensions |
| Deterministic report | Educational explanation accompanying computed scores; no generated narrative |
| AI report | Validated provider-generated prose accompanying authoritative computed scores |
| Generation attempt | One admitted provider generation workflow, including failures and internal retries |
| Refresh family | Rotating sessions revoked together on replay or logout |

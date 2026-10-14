# Decisions, risks, remaining work, and agent constraints

Source review date: **2026-10-14**. This record distinguishes delivered source from remaining product and operational decisions; it does not declare production readiness.

Read [Overview](01-overview.md) for delivered scope, [Architecture](03-architecture.md) for contracts, [User guide](02-user-guide.md) for setup, and [Development verification](04-development.md#verification-results) for executed checks. Delivery status and remaining decisions are maintained in these documents.

## Architecture decisions

| Decision | Selected behavior and reason | Consequences and limits |
| --- | --- | --- |
| Modular Go backend and React browser client | Shared account/scoring/storage contracts with real API data | One backend and synchronous bounded generation; no queue or distributed generation worker |
| Same-origin API | Keep cookies, OAuth return pages and exact Origin checks consistent | Vite/Nginx proxy API; private backend/database in Compose; public HTTPS remains operator work |
| Explicit immutable SQL | Preserve constraints and upgrade integrity | Embedded checksum ledger/transactional advisory lock; add forward migrations; no AutoMigrate |
| Current-state authentication | JWT alone cannot reflect disablement/revocation | Current user/session/version/role checks on every request; extra database access |
| Strict rotating refresh | Replayed values must invalidate related sessions | Hashed values, user-first locking, committed family revocation; no grace window; fallback clients need refresh coordination |
| In-memory browser access JWT | Avoid persistent client token storage | HttpOnly cookie restores sessions; pending refresh/Web Locks coordinate; preference-only localStorage |
| Exact Origin and bounded proxy | Establish cookie/admin mutation and client-IP trust boundaries | No wildcard; stricter Origin on mutations; Compose trusts Nginx only and proxy replaces IP headers |
| No implicit Google linking | Matching email does not prove merging permission | Subject identity; conflict uses existing login; explicit linking remains future work |
| Role-aware development defaults | Make local user/admin workflows usable without manual SQL | Default seeding on in dev/test and off in production; insert only missing roles, never reset/promote; published passwords require private production replacement |
| Explicit first-admin command | Preserve deliberate initialization outside automatic development defaults | Separate idempotent command; existing admin prevents changes; production opt-in requires private credentials; recovery remains operator work |
| Original bilingual banks | Reflection needs explicit wording and scoring versions | Immutable applied content; original constructs are not clinically validated; norms/reliability/equivalence remain unknown |
| Unknown remains null | Do not turn missing knowledge into numerical evidence | Separate unknown/omitted, exact two-thirds sufficiency, null gaps preserved in reports/charts |
| Descriptive compatibility | Compare answer ranges without forecasting a relationship | Mean similarity over sufficient shared dimensions, at least three; asserted consent/independence; no probability interpretation |
| Owner drafts and revisions | Save/resume without cross-user access or silent stale overwrite | Owner predicates and whole snapshots; conflicts; completed immutable; 100 active drafts |
| Deterministic results independent of AI | Provider credentials/costs must not prevent scoring | Local/insufficient/no-provider reports stay honest; failures retain scores/fallback rather than invented narratives |
| Separate external AI-sharing consent | Saving locally and transmitting externally are different actions | Persist consent_at/ai_consent_at; external AI false by default; unconsented/proxy compatibility never sent; explicit local examples need no external-sharing consent |
| Truthful local examples and preview | Exercise report UI without credentials or pretending a live model ran | Kind example with localized notice and exact score echoes; pure local submission ignores live-config races; admin preview uses disclosed sample defaults and never reads live keys |
| Atomic UTC admission quotas | Concurrent external generation must respect global/user attempt budgets | Global then user locks; atomic rollback; failed attempts count/retries share attempt; explicit local examples/previews bypass quotas; legacy configured-mock generation retains standard path |
| External work outside locks | Provider latency must not hold account/quota transactions | Commit processing then generate; final state/account checks; recovery fails abandoned attempts |
| One configured provider/fixed hosts | Avoid credential/context transmission to arbitrary URLs | Four adapters/no redirects or cross-provider fallback; model format validation without entitlement discovery |
| AES-256-GCM live-provider keys | Database credential reads should not reveal plaintext key | Nonce/version/associated data; master key needed for recovery; mock is key-free and clears prior encrypted key; answers/reports remain separate sensitive plaintext |
| Structured reports and exact echoes | AI must not replace scores or invent reference IDs | Required schema/nullable echoes/references/bounds; narrative meaning is not fully semantically verified |
| Restricted real administration | Useful controls without routine private answer access | Aggregate analytics/narrow users/audits; protected admins; password/email-confirmed erasure |
| Bounded authentication cleanup | Limit expired auth rows without weakening refresh replay detection | Five-second batches; user-first locks; live-family expired ancestors retained; not a report-retention policy |
| Operational deletion metadata | Remove sensitive data while preserving operational evidence | Cascades/null references/user-counter deletion; global history survives; admin-erasure UUID may remain in audit |

Evidence is in [Architecture](03-architecture.md), [default accounts](../backend/internal/auth/defaults.go), [analysis](../backend/internal/analysis/service.go), [AI](../backend/internal/ai/service.go), [local examples](../backend/internal/ai/example.go), [admin](../backend/internal/admin/service.go), [maintenance](../backend/internal/maintenance/service.go), [frontend](../frontend/src/main.tsx), and [Compose](../docker-compose.yml). These are current source choices. Where earlier notes did not record comparative alternatives, historical rationale remains unknown.

## Remaining work and open questions

The October 14 review found the core reflection/account/admin workflows implemented, corrected session refresh failure handling, and passed local backend/frontend checks plus all four browser scenarios in one run. No additional core implementation task was identified. The remaining items below distinguish external release acceptance from optional product extensions.

### External operation and release acceptance

- Live Google and intended paid-provider model/accounts need real credentials and runtime acceptance. Offline adapter/identity fixtures verify local contracts, not account entitlement or current service behavior.
- Docker/Compose and native Linux need an actual target runtime check. Cross-builds and a committed CI definition do not establish a successful Linux deployment.
- HTTPS ingress, certificates, hosting/incident owner, supported OS/browser matrix, rollout and recovery targets remain operator decisions. Production configuration checks do not provision these systems.
- Development account defaults are intentionally public. SEED_DEFAULT_ACCOUNTS false does not remove/change accounts seeded earlier; a production database must not retain those public credentials. Production automatic-seeding opt-in requires both private credential pairs.
- Storage encryption/access control, provider data-processing terms, report/audit retention, backup rotation and master-key recovery need explicit policy. Automatic age-based report erasure is not assumed.
- The repository is [pedram95kzm/relation-manager](https://github.com/pedram95kzm/relation-manager), using `main`. License, release and contribution choices remain with the user; publishing source does not establish production acceptance or a passing remote CI result.

### Product features outside the delivered workflow

- Email verification, password recovery, MFA, explicit Google linking and fresh-Google self-erasure are absent. Administrators can erase ordinary Google-only accounts after their own password reauthentication and target-email confirmation.
- Partner invitations/independently authenticated respondents are absent. Sharing consent and origin are assertions inside one owner's analysis, not verified two-account consent.
- Only questionnaire/scoring version 1.0.0 is retrievable/computable. Version fields alone do not implement historical dispatch. Preserve applied content and add explicit dispatch/tests before another version.
- Completed analyses are immutable. Failed/insufficient results can be reviewed/edited in the wizard and resubmitted; failed results also offer deterministic completion. Resume chooses the first unfinished page from stored answers/context rather than storing a separate step index.
- UI/question wording switches language; saved report narratives retain submission locale. Changing preferences does not implicitly translate/regenerate a report.
- Live model IDs have conservative format validation, without entitlement/model discovery. Mock uses the fixed local-example-v1 template. It demonstrates UI/report contracts, not live model quality or account access; there is no currency-cost calculator or hard spending cap.
- Throttles remain per-process. Report/audit/quota retention and distributed protection are not a completed multi-replica operations design.

## Known risks and practical limits

| Risk | Current control and residual implication |
| --- | --- |
| Clinical/statistical overinterpretation | Educational framing, null gaps and similarity disclosure; no clinical validity, norms, reliability study or cross-language equivalence |
| Narrative errors/prompt injection | Trusted prompts/typed bounded data/no tools/schema/echo/reference/text checks; not a guarantee about every statement or recommendation |
| Unsafe conflict guidance | No/unknown safety requires caution/support shape; prompts prohibit scripts/confrontation; deterministic fallback is static caution; generated meaning still warrants care |
| Unverified partner permission | Unconsented/proxy comparisons suppress index/AI; an owner's checkbox does not establish another identity or independent permission |
| Local examples mistaken for real AI | Example kind/provider/notice and sample-input flags distinguish local templates; displayed echoes are real arithmetic but narrative is illustrative and preview defaults are not user measurements |
| Cost beyond nominal quota | Atomic attempt admission protects concurrency; retries can make several paid requests and admin tests are separate; no token/currency guarantee |
| Public development accounts | Startup inserts missing roles without resets; production opt-in needs private passwords; previously seeded rows must be changed/erased intentionally before exposure |
| Refresh races | Replay revokes family; pending promise/Web Locks reduce browser races; unsupported browsers/external clients must coordinate |
| Secret/key loss | Setup preserves existing values/encrypted provider key; master-key change can strand ciphertext; signing-key change alone does not revoke refresh |
| Database/backup disclosure | Owner APIs/deletion cascades restrict application access; database operators/dumps expose sensitive plaintext; storage protection/restoration remain external |
| Retained deleted identifier | Nullable references clear/narratives erase; admin deletion audit can retain UUID, requiring operational-retention policy |
| Mid-generation config changes | Transactions protect admission/finalization but provider work is synchronous; settings changes affect subsequent calls and abandoned reservations stay consumed |
| Distributed operations | Process throttles/fixed proxy trust fit supplied topology; more proxies/replicas need IP/rate-limit review |
| Upgrade integrity | Drift fails intentionally; do not bypass ledgers/delete real data; reviewed forward corrections and protected recovery required |
| Target/provider compatibility | Definitions and fixtures are evidence, while live target/account acceptance is separate; see verification ledger |

## Constraints for future AI agents

1. Inspect source, current scope and repository changes before editing. Current implementation authorization supersedes old documentation-only notes; later user authorization remains authoritative.
2. Preserve applied migration checksums and questionnaire content. Add forward migrations/explicit versions rather than editing history, using AutoMigrate, clearing ledgers or deleting real data.
3. Preserve current-state JWT/session/role checks, user-first locking, rotating refresh hashes and committed replay-family revocation. Never serialize/log passwords, hashes, refresh values, keys or raw sensitive request bodies.
4. Preserve exact Origin on cookie/admin mutations, host-only HttpOnly cookies, production HTTPS/secure-cookie requirements and bounded trusted proxies. New ingress must establish trustworthy forwarded client-IP behavior.
5. Never implicitly link, promote or reset by matching email. Default seeding inserts only absent roles and explicit seed-admin remains idempotent. Preserve production private-credential policy; do not reset accounts to the published development values. Admin targets stay protected; erasure needs confirmation/reauthentication.
6. Preserve strict JSON/type/duplicate/depth/body validation and safe errors. Derive ownership from authentication and predicate both analysis ID/owner; browser route guards do not authorize API access.
7. Preserve integer 1..5/null/omitted, reverse 6-x, exact two-thirds sufficiency, null insufficiency, deterministic precision and coverage. AI never calculates/overwrites scores.
8. Preserve original/nonvalidated framing and both application content languages. Published documentation must be English only. Do not claim clinical diagnosis, statistical confidence, population norms or relationship-success probability.
9. Preserve complete draft snapshots/revisions and final validation separate from scoring. Completed records stay immutable unless a reviewed lifecycle changes that contract.
10. Preserve separate persisted external AI consent, deterministic/no-provider outcomes, partner permission/origin suppression, atomic global/user quotas and failed-attempt accounting. Explicit local examples/previews stay free of outbound calls and external-sharing consent; never silently change their provider when live settings change. External calls remain outside transactions; the pure local generator may run inside the analysis transaction.
11. Keep live keys server-side/encrypted, endpoints finite/no redirects, trusted prompts separate from supplied context, and exact local echo/reference validation before persistence. Only the trusted local generator may emit kind example; external output must remain kind ai. Disclose synthetic preview defaults and never record answers/context/report in preview audits. Schema checks are not semantic safety proof.
12. Recheck account/state/revision when finalizing; keep stale recovery and expired-auth cleanup without removing live-family replay ancestors; never resurrect deleted/recovered records. Label fallback deterministic.
13. Use real admin aggregates and minimal operational metadata. Never fabricate analytics or add private answer/report browsing without a justified explicit contract. Disclose retained identifiers accurately.
14. Use disposable PostgreSQL databases named with test. Missing TEST_DATABASE_URL means integration skips. Separate live credentials/services, containers and target OS checks from fixtures/cross-builds.
15. Keep one index/five main docs, update contracts in their primary homes and verification in [Development](04-development.md#verification-results). Do not stage/commit/push/publish/select a license/change remote without relevant user authorization.

## Retained product roadmap

Original phase structure remains a useful map. Source status and runtime acceptance are separate; this table does not imply every possible enhancement is complete.

| Phase | Delivered source and remaining acceptance |
| --- | --- |
| 0: Inspect/plan | Consolidated docs/source audit and current implementation scope |
| 1: Foundation | Configuration/migrations/logs/health/Compose/startup scripts; target/container runtime separate |
| 2: Models/API | Constraints, strict JSON/errors/pagination and forward AI-consent migration |
| 3: Accounts | Sessions/profile/security, Google/browser callback, role-aware development defaults, explicit seed/admin erasure, bounded auth cleanup; live Google/recovery/linking limits remain |
| 4: Banks/scoring | Original bilingual definitions/deterministic tests; single-version limit |
| 5: AI | Four live adapters, local example/preview mode, six prompts, encryption/validation; live provider/model acceptance remains |
| 6: Analyses | Owner drafts/history/submit/report/delete, revisions, separate external consent, quotas, free local examples, fallback/recovery |
| 7: React | Same-origin auth client/routing, bilingual RTL/LTR, light/dark shell |
| 8: User flows | Wizards/review/save/resume, tables/bars/nulls/history/results/print; step-index/translation limits |
| 9: Administration | Real aggregates/user disable/limits/erasure, live connection tests, independent local previews, provider config/audit |
| 10: Verification | Backend/HTTP/PostgreSQL/frontend/browser tests and CI; executed evidence stays in ledger |
| 11: Operations | Three-service Compose/Nginx/CSP, setup/run/verify/backup scripts and English docs; public TLS/policies/target acceptance remain |
| 12: Acceptance | Available local evidence recorded; no blanket production-ready/external-success claim |

## Documentation history

Earlier planning notes were consolidated into this six-document set; ignored local archives preserve them without becoming a public source of truth. The previous audit accurately described a backend-only checkout at that time. Obsolete frontend/AI/analysis/admin absence claims and documentation-only restrictions are replaced with current source-backed contracts.

Documentation must distinguish implementation, executed verification and unresolved external/operational choices. A field, code path, workflow or passing fixture is not successful live-deployment evidence by itself.

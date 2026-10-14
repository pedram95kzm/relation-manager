# Development and verification

Reviewed **2026-10-14**. Setup is in [User guide](02-user-guide.md); actual routes/security are in [Architecture](03-architecture.md); invariant decisions are in [Decisions](05-decisions.md).

## Repository structure

```text
relation-manager/
  backend/
    cmd/server, cmd/seed-admin
    internal/auth, config, database, httpapi, platform
    internal/questionnaire/definitions, scoring
    internal/analysis, ai/prompts, admin, maintenance
    migrations/0001_foundation.*, 0002_domain.*, 0003_analysis.*, 0004_ai_mode.*
    go.mod, go.sum, Dockerfile
  frontend/
    src/pages, src/components
    src/api.ts, auth.tsx, i18n.tsx, types.ts, main.tsx, style.css
    tests/fullstack.spec.ts, playwright.config.ts
    package.json, package-lock.json, vite.config.ts, tsconfig.json
    Dockerfile, nginx.conf
  scripts/
    setup-env, load-env, run-backend, verify, backup (.ps1/.sh)
  .github/workflows/ci.yml
  docs/ (index plus five documents)
  docker-compose.yml, .env.example, README.md, LICENSE
```

`.local/` is ignored private storage for local runtime environments, owned disposable clusters, verification outputs, and dumps. `node_modules`, `dist`, traces/screenshots, binaries, and `.env` are ignored. Do not distribute a local database cluster or report data with application source.

## Native development

Go 1.27+, Node 22.12+, and PostgreSQL are required. The implementation host used Windows amd64, PowerShell 5.1, Go 1.27.1, PostgreSQL 15.5, Node 24.15.0/npm 10.9.2, and installed Chrome. Linux builds were cross-compiled; native Linux/container runtime was unavailable locally.

From the root, prepare `.env` and a native database as in the User guide, then use `scripts/run-backend.ps1` or `bash scripts/run-backend.sh`. In another terminal, `cd frontend`, `npm ci`, and `npm run dev`. Vite uses port 5173 with strict port selection and proxies `/api` to 127.0.0.1:8080. Use `API_PROXY_TARGET` if the API address changes.

For manual Go commands in PowerShell:

```powershell
. ./scripts/load-env.ps1
Set-Location backend
go run ./cmd/server migrate
go run ./cmd/server
```

For Bash:

```bash
source scripts/load-env.sh
cd backend
go run ./cmd/server migrate
go run ./cmd/server
```

Loaders parse literal values; they do not execute `.env` as shell code. Direct Go commands do not auto-load a file. Stop/rebuild/restart for backend changes. Frontend development uses Vite updates; production uses `npm run build` and the static proxy container. Development/test startup initializes missing admin/user roles unless disabled. The separate explicit admin seed remains idempotent; migrations-only execution creates no default accounts.

Standalone native builds from `backend/`:

```bash
go build -o ../.local/relations-server ./cmd/server
go build -o ../.local/relations-seed-admin ./cmd/seed-admin
```

On Windows use `.exe` output names. From PowerShell, set `CGO_ENABLED=0`, `GOOS=linux`, and `GOARCH=amd64` only in the build process environment for cross-builds; remove them afterwards before native commands. Cross-build success is not Linux execution.

## Build, formatting, and tests

From `backend/`:

```bash
go mod verify
gofmt -l cmd internal migrations
go vet ./...
go test ./... -count=1
go build ./...
```

`gofmt -l` should print nothing. Use `gofmt -w` on changed Go files. From `frontend/`:

```bash
npm ci
npm test
npm run typecheck
npm run build
npm audit --audit-level=high
```

Frontend dependencies are exact-pinned with a lockfile and HTTPS registry. Prettier is installed for formatting; `npx prettier --write src vite.config.ts playwright.config.ts tests index.html package.json tsconfig.json` formats application source. Local font assets are bundled, with no runtime request to an external font service. Vitest covers refresh coordination, retry/failure behavior, proactive refresh denial, session preservation across temporary network/server failures, memory-only tokens, and null-vs-zero accessible score tables. Browser tests cover complete user/admin workflows.

Convenience checks from the root:

```powershell
$env:TEST_DATABASE_URL = Read-Host 'Dedicated disposable test PostgreSQL URL'
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/verify.ps1 -RequireDatabase
```

```bash
read -r -s -p 'Dedicated disposable test PostgreSQL URL: ' TEST_DATABASE_URL
printf '\n'
export TEST_DATABASE_URL
bash scripts/verify.sh --require-database
```

These scripts verify Go modules/formatting/vet/tests/build and run frontend locked installation/tests/build. They do not start a database/browser or prove live integrations. Use a fresh, owned, disposable PostgreSQL database whose name contains `test`; never a private/shared/production database. Tests skip PostgreSQL checks without `TEST_DATABASE_URL`. Integration fixtures use isolated schemas or rollback; migrations/catalog may persist in the dedicated test target. Remove test env values after use.

### Test coverage

| Area | Meaningful exercised cases |
| --- | --- |
| Config/database | Ranges, production policy, immutable checksums, SQL constraints, JSONB objects, rollback, cascades |
| Authentication | Password/account/profile flows, revocation, disabled state, token claims, refresh replay/concurrency, admin seed |
| Google | Offline code exchange/JWKS/claim fixtures, single-use state, PKCE, nonce, email-link refusal, safe redirects |
| Catalog/scoring | Original banks/public projection, context/answer bounds, immutable seed drift, missing/null/reversal/sufficiency/normalization/similarity |
| Analysis | Owner predicates, strict DTOs, optimistic revisions, final validation, separate consent, 100-draft cap, immutable completion, failure/retry |
| Quotas/recovery | Atomic concurrent global/user limits, per-user override, reservation rollback, UTC rollover, interruption and account disable/deletion mid-generation |
| AI | Four HTTP protocol fixtures, bounded total retry budget, redirect refusal, malformed/truncated/refused reports, exact numeric/null echoes, references/safety, finite prompts, mock/preview denying-transport checks, explicit local examples with active live config/exhausted quotas |
| Encryption/admin | AES-GCM randomness/tampering/wrong key, secret-free output/audit, current role/Origin, default-account no-reset/no-promotion initialization, real aggregates, user limits/disable, reauthenticated ordinary/Google-only erasure |
| Maintenance | Bounded cleanup, deadline/cancellation, user locks, live-family ancestor preservation and replay revocation |
| Frontend | Refresh request coalescing, reactive/proactive denied refresh, temporary network/503 recovery without logout, bearer retry, token storage, score table null vs zero |
| Browser | Registration/profile, all three wizards, explicit unknowns, exact scores/similarity, autosave/resume/delete/history, RTL/theme/session reload, admin denial real admin settings, default account logins, and local example generation |

Race checks require a compatible CGO compiler. The local Windows C compiler previously lacked 64-bit support; local race validation is not claimed. Linux CI declares `go test -race ./...` with PostgreSQL. No comprehensive load/penetration study or psychometric validation is implied by tests.

### Browser integration tests

Start a disposable native/Compose stack first. From `frontend/`, either install Playwright Chromium with `npx playwright install chromium`, or use an existing compatible browser executable. Example PowerShell:

```powershell
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
$env:PLAYWRIGHT_BASE_URL = 'http://localhost:5173'
npm run test:e2e
```

For administrator scenarios, initialize a test-only admin and set `E2E_ADMIN_EMAIL`/`E2E_ADMIN_PASSWORD` in the test process environment. Without those values, the admin scenarios are explicitly skipped. Set `E2E_DEFAULT_ACCOUNTS=true` to verify development default logins; the container CI job uses a fresh disposable development database and the documented default credentials to run all four cases. Browser scenarios create records and edit provider/limit configuration; never run them against a shared application. Restart the disposable API between repeated full suites if the five-per-hour registration throttle is exhausted. Tests are sequential, report actual page exceptions, and check mobile overflow.

Playwright output is ignored. Source changes/reloads during a browser run can reset component state; run acceptance against settled source or built containers. Passing an earlier intermediate run does not substitute for the final acceptance run.

For local browser checks against built assets, stop the owned Vite development server, run `npm run build`, then `npm run preview`. Preview uses port 5173 and inherits the same API proxy, avoiding development module/HMR requests. It is a local verification server; production uses the supplied static proxy container or an operator's equivalent.

## Continuous integration

[GitHub Actions](../.github/workflows/ci.yml) declares:

- PostgreSQL-backed Go module checks, formatting/vet, race tests, and builds.
- Frontend locked install, unit tests, production build, and dependency audit.
- Compose build/readiness, missing-role development defaults in a fresh disposable database, Chromium install, and four user/admin/default-account/local-example browser checks.

The workflow was source/YAML checked locally. Publishing to the configured GitHub repository triggers CI; its result must be checked separately, and no successful remote workflow result is claimed here. Docker executable was unavailable locally, so container execution remains externally unverified until CI or an operator executes it.

## Migrations and versioned content

SQL files in [migrations](../backend/migrations) are authoritative. The migration runner applies sorted embedded up files in a transaction under an advisory lock and verifies applied SHA-256 checksums. `0003_analysis` adds separate AI consent; `0004_ai_mode` expands provider constraints for key-free mock mode. Earlier migrations remain unchanged. Never use AutoMigrate or clear checksum records to repair drift.

Add a new numbered migration for schema changes. `RollbackLast` is a development/test library helper, with no production rollback CLI. Models must preserve explicit table names, UUID/FK/check constraints, and JSONB object serialization. Production recovery needs reviewed forward correction or a tested separate restore.

Questionnaire and scoring versions currently equal `1.0.0`. Preserve wording/reverse metadata for an applied version. Catalog lookup/validation still dispatch one version, so introducing another version requires explicit catalog/scoring code and tests. Stored results preserve versions and are never recomputed silently on read.

## Questionnaire and scoring contracts

Answers are respondent-to-question maps of integer 1..5 or null. `self` is always allowed; `partner` only for compatibility. Missing keys mean omission; present null means explicit unknown. Draft validation permits incomplete data; submit separately applies final required-choice validation. Context fields are module-specific, strictly typed, and bounded.

```text
keyed = x (forward) or 6-x (reverse)
minimum numeric responses = max(2, ceil(2*n/3))
numeric completeness = answered / eligible items
score = 25 * (keyed numeric mean - 1)
```

Insufficient score/band are null. Bands use unrounded values: below 40 lower, 40..60 middle, above 60 higher. Display rounds to one decimal without changing stored arithmetic. Self requires three sufficient dimensions; conflict can be context-led with null style scores.

Compatibility components are `100-abs(A-B)`, equally averaged only with three sufficient shared dimensions, independent origin, and asserted partner sharing consent. Coverage is shared sufficient dimensions/15. Proxy or unconsented data never enables compatibility AI transmission. Individual scores remain available when the similarity index is withheld. Dimensions do not sum to 100 and are displayed as separate bars/tables.

The finite references motivate constructs, not validation of original items:

| ID | Primary research reference |
| --- | --- |
| `bfi2` | [Soto and John, 2017](https://doi.org/10.1037/pspp0000096) |
| `attachment` | [Fraley, Waller and Brennan, 2000](https://doi.org/10.1037/0022-3514.78.2.350) |
| `emotion_regulation` | [Gross and John, 2003](https://doi.org/10.1037/0022-3514.85.2.348) |
| `emotion_awareness` | [Gratz and Roemer, 2004](https://doi.org/10.1023/B:JOBA.0000007455.08539.94) |
| `communication` | [Christensen and Heavey, 1990](https://doi.org/10.1037/0022-3514.59.1.73) |
| `conflict_styles` | [Kilmann and Thomas, 1977](https://doi.org/10.1177/001316447703700204) |
| `relationship_similarity` | [Dyrenforth and colleagues, 2010](https://doi.org/10.1037/a0020385) |

No licensed instrument reproduction, population norm, clinical reliability, or Persian/English measurement-equivalence claim is made. Questionnaire files are the canonical wording; metadata/references stay on the backend. AI output must match authoritative scores exactly, with explicit nulls and finite reference IDs.

## Logging, backup, and upgrade review

Use live/ready endpoints and request IDs to distinguish process/database issues. Logs include method/route/status/latency; password/token/provider/body data is not routinely logged. Review startup migration error text before sharing. Production adds HTTPS cookie/origin/database policy, not automatic TLS installation.

Backup scripts create unique custom-format dumps, preserve secrets outside command arguments for native backup, and validate native listings. Dumps remain sensitive unencrypted data. Keep environment/master-key recovery material separately. A native dump was restored into a newly created separate disposable database and all three catalog rows verified. Container backup/restore was not executed locally. Report retention/backup scheduling is an operator policy, not an inferred implementation.

Before an upgrade, inspect migrations/version compatibility, create/test a protected backup, install locked dependencies, run applicable tests/builds, restart/recreate, and verify readiness. Preserve Compose volume/project identity. An older executable may be incompatible with newer migrations. Never test a restore by replacing live data.

## Git and contribution workflow

The canonical repository is [pedram95kzm/relation-manager](https://github.com/pedram95kzm/relation-manager). The `origin` remote uses `https://github.com/pedram95kzm/relation-manager.git`, and the primary branch is `main`. The initial import includes source, tests, locked dependencies, configuration templates and documentation; local runtime data and generated artifacts are ignored. Inspect untracked files as well as diffs before staging; ordinary `git diff` does not show untracked source.

```bash
git status --short
git branch --show-current
git remote -v
git diff --check
git check-ignore .env .local/example.dump frontend/node_modules/example backend/server.exe
```

The source uses the [MIT License](../LICENSE), copyright (c) 2026 pedram95kzm. Release tags, contribution and review policy remain undecided. Keep source, lockfiles, migrations, Compose, `LICENSE`, and `.env.example` tracked; keep private config/dumps/data/artifacts ignored. Applied migrations/catalog and security/scoring invariants require tests and corresponding docs updates. Never include secret values or sample personal reports in a change description.

## Verification results

Latest evidence date **2026-10-14**, Windows amd64, Go 1.27.1, Node 24.15.0/npm 10.9.2, PostgreSQL 15.5, and installed Chrome. The latest checks used a new owned loopback cluster on port 55434 with separate disposable databases `relations_test_final14` for integration tests and `relations_test_browser14` for browser workflows. The API listened on 127.0.0.1:8084 and the built frontend preview on localhost:5173. Existing PostgreSQL services/private data and earlier local clusters were untouched. Backup/script/image-tag checks originated on October 11; cross-build and standalone preview/API checks below were last executed on October 13.

| Check | Executed result and limit |
| --- | --- |
| Go unit/integration suites | PASS (October 14): `go test ./... -count=1` with real PostgreSQL, including default initialization, offline live-adapter fixtures, mock/preview, explicit local examples, maintenance and existing packages |
| Go module/vet/native build | PASS (October 14): `go mod verify`, clean `gofmt -l`, `go vet ./...`, `go build ./...` and a native server executable |
| Linux server/seed cross-build | PASS (October 13): Linux/amd64 server and seed-admin cross-builds with CGO disabled; native Linux runtime remains unverified |
| Frontend unit tests | PASS (October 14): 7 tests across 2 files; the 3 new refresh regressions failed before the correction and passed afterwards |
| Frontend production build/typecheck | PASS (October 14): `npm run build` runs `tsc -b` and Vite; locally served fonts and static assets bundled |
| npm audit | PASS (October 14): full `npm audit --audit-level=high` reported zero vulnerabilities at execution time |
| Native full-stack browser checks | PASS (October 14): all 4 scenarios in one clean 38-second built-preview run, with no skips: user/all modules/history/mobile logout, administration, startup default logins, and local previews/example/no-quota submission |
| Native local preview/example API | PASS (October 13 standalone API checks; October 14 browser scenario): all three preview modules return kind example/external_request false; ordinary-user explicit local submit completes with no AI consent and zero quota |
| Script validation | PASS (October 11 syntax/no-overwrite/native backup); setup/load also executed with separate configuration on October 14 |
| Native backup/restore | PASS (October 11): restored into fresh separate database; three questionnaire rows verified |
| Compose/CI | YAML/topology/configuration reviewed; official image tags reachable; Docker runtime and remote CI not executed locally |
| Documentation audit | PASS (October 14): six-document layout plus root README, 129 relative links/anchors, 31 code blocks, English-only text, final newlines/whitespace, obsolete checklist removal, private-secret isolation and ignored verification artifacts. Configuration/all 34 API routes were reconciled in the prior source audit |
| Race detector | Local compatible 64-bit C toolchain unavailable; Linux CI declares race checks but no remote result claimed |
| Live Google/AI | Not executed; no live credentials supplied |
| Production TLS/rollout/load/penetration review | Not executed; no production readiness claim |

The October 14 source review corrected inconsistent refresh failures: proactive 401 rejection previously left the browser signed in, while temporary failures during reactive refresh cleared its session. Denial handling now belongs to the shared refresh operation; temporary failures propagate without logout and permit another attempt. Regression tests demonstrated both prior failures. No additional missing core workflow was found; external acceptance and optional extensions remain in [Decisions](05-decisions.md#remaining-work-and-open-questions).

The clean October 14 browser run supersedes the October 13 split-run limitation caused by host socket exhaustion. Earlier desktop English/mobile Persian screenshots remain only under ignored `.local/delivery13/`; October 14 configuration, database, and logs are under ignored `.local/final14/`. The owned API, preview, and test database processes were stopped after verification. The separate CI container job declares all four cases but was not executed remotely here.

Execution limits do not mean implemented adapters/UI are absent. Conversely, passing source/local checks does not establish external account access or production acceptance.

# User guide

This guide describes the implemented full stack as of **2026-10-14**. All documentation is English. The browser interface supports English and Persian. See [executed verification](04-development.md#verification-results) for platform and external-service limits.

## Start here

Run the Go API and React frontend against PostgreSQL, or use the three-service Compose stack. Open **http://localhost:5173**. The API on port 8080 returns JSON; its root is not a website. Deterministic results work without provider credentials. Google and AI are optional integrations; an application encryption key is still mandatory at startup.

Prerequisites for native development: Go 1.27+, Node.js 22.12+ (Node 24 tested), PostgreSQL, and network access to download dependencies. PostgreSQL client tools are needed for database creation and native backup. Docker is needed only for the container route. Git is optional for running a downloaded checkout.

## Installing and running on Windows

PowerShell 5.1, Go 1.27.1, Node 24.15.0/npm 10.9.2, and PostgreSQL 15.5 were exercised on Windows amd64. Install prerequisites from [Go](https://go.dev/doc/install), [Node.js](https://nodejs.org/en/download), and [PostgreSQL](https://www.postgresql.org/download/windows/). Match paths to your installation.

From the repository root:

```powershell
$taskPgBin = 'C:\Program Files\PostgreSQL\15\bin'
$env:Path = "$taskPgBin;$env:Path"
go version
node --version
npm --version
pg_isready -h 127.0.0.1 -p 5432
```

For a new native database only, create a dedicated role/database using appropriate PostgreSQL administrative access. Skip creation if they already exist:

```powershell
createuser -h 127.0.0.1 -p 5432 -U postgres --pwprompt relations
createdb -h 127.0.0.1 -p 5432 -U postgres --owner=relations relations
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/setup-env.ps1
notepad .env
```

Set `DATABASE_URL` to the database you created, with its actual URL-encoded role password. The setup script generates `JWT_SECRET`, `AI_ENCRYPTION_KEY`, and `POSTGRES_PASSWORD`; it never overwrites an existing file. The container password is separate from an existing native PostgreSQL role password. File values must be unquoted `KEY=value` lines; restrict file access locally.

Keep `PUBLIC_URL=http://localhost:5173`, `CORS_ORIGINS=http://localhost:5173`, `HTTP_ADDR=127.0.0.1:8080`, and development cookies for this local setup. Use localhost consistently in the browser.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/run-backend.ps1
```

In a second terminal, from the root:

```powershell
Set-Location frontend
npm ci
npm run dev
```

Open [Relations](http://localhost:5173). Vite forwards `/api` to the backend at 127.0.0.1:8080. Stop either process with Ctrl+C. Restart the API after Go/config changes; Vite updates frontend changes during development. For a different API port, set `API_PROXY_TARGET` in the frontend terminal, and keep browser Origin/Public URL aligned with the frontend address.

The backend migrates, registers immutable question banks, initializes missing quota settings, and recovers abandoned processing records at startup. Explicit migration is available with `scripts/run-backend.ps1 -Command migrate`. Scripts load `.env`; direct `go run` commands do not.

## Installing and running on Linux

Native Linux execution was not performed locally. These Bash commands target a typical Ubuntu installation; Linux cross-builds passed. Install Go/Node/PostgreSQL through their official instructions and ensure PostgreSQL is running.

For a new role/database on a standard local PostgreSQL setup:

```bash
sudo -u postgres createuser --pwprompt relations
sudo -u postgres createdb --owner=relations relations
bash scripts/setup-env.sh
```

The setup script requires OpenSSL, creates secrets with restrictive file permissions, and preserves an existing `.env`. Edit `DATABASE_URL` for your native database and keep the frontend origin at localhost:5173. The loader parses literal unquoted values without executing shell expressions.

```bash
bash scripts/run-backend.sh
```

In another terminal:

```bash
cd frontend
npm ci
npm run dev
```

Open http://localhost:5173. Ctrl+C stops each foreground process. Native database files remain in PostgreSQL. To invoke Go manually, load environment with `source scripts/load-env.sh` from the root before entering `backend/`.

## Docker Compose on Windows or Linux

Requires Docker with Linux containers and the Compose plugin. Docker was unavailable on the implementation host; repository CI declares a full container smoke/browser check. Do not interpret source inspection as an executed deployment.

Prepare `.env` using the matching setup script above. `POSTGRES_PASSWORD` must be nonempty and URL-safe; Compose constructs its internal database URL from it. Default frontend publication is **127.0.0.1:5173**. No API/database host ports are published.

```bash
docker compose config --quiet
docker compose up --build --wait
docker compose ps
docker compose logs --tail=100 backend database frontend
docker compose exec backend wget -q -O - http://127.0.0.1:8080/api/v1/health/ready
```

Open http://localhost:5173. The frontend serves the SPA and proxies `/api` on the same origin. `WEB_PORT`/`WEB_BIND_ADDR` control publication; when changing the browser port/origin, also update `PUBLIC_URL` and `CORS_ORIGINS`.

```bash
docker compose run --rm backend migrate
docker compose stop
docker compose start
docker compose restart backend
docker compose up --build --wait
docker compose down
```

Use `up --build` after code changes and `up -d --force-recreate backend` after environment changes. `restart` does not reload container environment. `down` preserves `postgres_data`; do not add volume deletion flags. Keep the Compose project/folder stable. Changing `POSTGRES_PASSWORD` does not rotate an existing database role's password.

## Configuration

The authoritative definitions are [config.go](../backend/internal/config/config.go) and [.env.example](../.env.example). Secrets belong only in protected server configuration. Never paste them into documentation, browser source, logs, or Git.

| Setting | Default/requirement and purpose |
| --- | --- |
| `APP_ENV` | development; development/test/production |
| `HTTP_ADDR` | Code default :8080; sample/native script 127.0.0.1:8080; Compose :8080 |
| `PUBLIC_URL` | http://localhost:5173; exact browser origin and Google return destination |
| `DATABASE_URL` | Required PostgreSQL URL; Compose replaces with private database URL |
| `POSTGRES_PASSWORD` | Required only for Compose; generated URL-safe password |
| `WEB_BIND_ADDR`, `WEB_PORT` | Compose-only defaults 127.0.0.1, 5173 |
| `JWT_SECRET` | Required generated secret, at least 32 characters |
| `AI_ENCRYPTION_KEY` | Required standard base64 of 32 random bytes; encrypts stored provider keys |
| `ACCESS_TOKEN_TTL`, `REFRESH_TOKEN_TTL` | 15m (1m..1h), 720h (1h..2160h) |
| `COOKIE_SECURE` | false in development; production requires true |
| `CORS_ORIGINS` | Defaults PUBLIC_URL; exact comma-separated origins, no wildcard/path |
| `TRUSTED_PROXIES` | Empty natively; Compose trusts only frontend proxy IP 172.30.0.10 |
| `AI_TIMEOUT`, `AI_RETRIES`, `AI_MAX_TOKENS` | Initial provider form defaults: 60s, 1, 4096; allowed 1..120s, 0..3, 512..16000 |
| `GLOBAL_DAILY_LIMIT`, `USER_DAILY_LIMIT` | Initial missing settings seed: 1000 and 10; ranges 1..1000000 and 1..10000 |
| `MIN_AGE` | 18; accepted setting 18..120 |
| `LOG_LEVEL` | info; debug/info/warn/error |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_CALLBACK_URL` | Optional; supply all three or none |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD` | Automatic admin overrides and explicit first-admin CLI credentials; local defaults below |
| `SEED_DEFAULT_ACCOUNTS` | Empty defaults to true in development/test and false in production; initializes missing admin/user roles at ordinary startup |
| `USER_EMAIL`, `USER_PASSWORD` | Default ordinary-user overrides for automatic role initialization |
| `API_PROXY_TARGET` | Frontend Vite process only; default http://127.0.0.1:8080 |

Restart/recreate the process after environment changes. Persistent admin limits/provider settings take precedence after initialization; changing environment limit defaults does not overwrite administrator changes. Provider keys/model selection are configured through the admin UI/API, not guessed environment variables.

Production configuration requires HTTPS public/CORS origins and secure cookies. Remote databases require `sslmode=verify-full`, except the private Compose hostname `database`. TLS ingress, external storage protection, monitoring, and release policy still need an operator. The local Compose file is not a certificate installer.

Changing `JWT_SECRET` rejects old access tokens but does not itself revoke stored refresh sessions. Changing `AI_ENCRYPTION_KEY` without re-encryption makes existing provider credentials unreadable; preserve the old key securely until an explicit migration/replacement plan is complete.

## First use and daily workflows

### Default development accounts

On ordinary server startup in development/test, these accounts are initialized only when their corresponding role has no existing account:

| Role | Default email | Default password |
| --- | --- | --- |
| Administrator | `admin@relations.local` | `RelationsAdmin2026!` |
| Ordinary user | `user@relations.local` | `RelationsUser2026!` |

Their profiles use eligible age `max(30, MIN_AGE)` and gender `prefer_not_to_say`, so reflections can start immediately. Initialization is transactional and idempotent. Existing accounts, including disabled accounts, prevent another default of that role; passwords, names, roles, and profiles are never reset or promoted. If a chosen default email belongs to another role, startup returns `default_email_unavailable`; choose a different configured email.

Override administrator credentials with `ADMIN_EMAIL`/`ADMIN_PASSWORD`, ordinary credentials with `USER_EMAIL`/`USER_PASSWORD`, or disable automatic initialization with `SEED_DEFAULT_ACCOUNTS=false`. `server migrate` does not initialize accounts. Production defaults automatic initialization to false; explicit opt-in requires all four private credential values and rejects either public demo password. Production configuration also rejects supplied public demo passwords when automatic initialization is disabled. Existing database accounts are not reset when the environment changes; change/replace development demo credentials before deploying that database.

### Completing a reflection

1. Create an account or sign in. Passwords must contain 12..72 UTF-8 bytes; a unique long password is recommended. An account requires an eligible age/gender profile before starting a reflection.
2. Use the language and theme controls in the header. Persian is the default; English uses LTR. Only language/theme preferences are stored in browser local storage. Responses remain in your server account, and access tokens stay in memory.
3. Choose self, compatibility, or conflict. Each questionnaire page contains up to six items. Select 1..5 or the explicit unknown choice; required items need one selection to continue. Unknown is different from an unanswered item and never becomes zero.
4. Drafts autosave after approximately 1.5 seconds and save on Continue/Save & exit. A failed save shows an error. Navigation/reload guards protect pending changes. Resume uses saved answers and starts at the first unfinished page. There is a cap of 100 active drafts per account.
5. Compatibility needs self/partner responses and context about origin/consent. Independent responses and asserted partner sharing consent are required for similarity and provider sharing; proxy estimates never enable those. Partner identity is not independently verified.
6. Conflict needs relationship type, reported safety, and a nonblank topic. Other narratives are optional and capped at 2,000 Unicode runes. A no/unknown safety response gets cautionary support language rather than confrontation scripts in the deterministic report.
7. Review responses. Educational storage/processing consent is required. AI sharing is optional, unchecked initially, and separately required before any provider call. The active provider receives computed scores, completeness metadata, and supplied context/narratives; choose carefully what you include.
8. View score tables/bars and limitations. Null dimensions appear as a dash. Numeric completeness is not confidence; similarity includes shared-dimension coverage and is not a success probability. AI prose supplements the computed numbers.
9. History can be filtered by module/status and paginated. Resume drafts, review/edit failed or insufficient records, delete records with confirmation, or print/save PDF from the result page. Completed records are immutable; create a new reflection to change them.

Without an active provider, submission produces a clearly labeled deterministic report. Insufficient data and suppressed compatibility still preserve available individual scores. Admitted AI failures preserve scores and an educational fallback, record a safe failure code, and consume one daily generation attempt. Failed records can be edited/retried with fresh consent. Quotas reset at **00:00 UTC**; the UI displays the recorded quota totals rather than inventing a local reset time.

Password change and logout-all revoke sessions. Change password from Account with the current password, then sign in again. Ordinary password users may erase their account after password reauthentication. Google-only self-erasure requires administrator help; administrator self-erasure is prohibited. Foreign-key deletion removes user references from operational events, while sanitized audit metadata can retain the deleted account UUID.

## Google OAuth

Supply all three Google settings from your OAuth project. Register the exact callback, normally `http://localhost:8080/api/v1/auth/google/callback` for native development or `http://localhost:5173/api/v1/auth/google/callback` through Compose. Hostname must match PUBLIC_URL; ports may differ for the direct API callback. Production uses HTTPS.

The Google button appears when enabled. On success the backend creates a refresh cookie and returns to `/auth/callback`; the frontend refreshes and opens the workspace. Errors return to login with safe codes. Equal email addresses do not implicitly merge identities; `account_link_required` means use the existing login method. Live credentials were not supplied, so live login remains unverified. Registration guidance: [Google web-server OAuth](https://developers.google.com/identity/protocols/oauth2/web-server).

## Administrator use and AI configuration

Development/test initializes missing roles automatically as described above. For a private initial administrator through the explicit CLI, set `SEED_DEFAULT_ACCOUNTS=false` before first ordinary startup, set private `ADMIN_EMAIL` and `ADMIN_PASSWORD` in `.env`, then run from the root:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/run-backend.ps1 -Command seed-admin
```

```bash
bash scripts/run-backend.sh seed-admin
# Containers use the executable override:
docker compose run --rm --entrypoint seed-admin backend
```

Seed creates the first admin only if none exists; it never resets passwords or promotes a matching ordinary email. Remove bootstrap password configuration afterwards. Sign in normally, then open Administration. Admin role alone does not bypass server authorization.

The Activity tab shows real aggregate counts and recent usage. Users supports search, disable/enable, individual daily limits, and reauthenticated ordinary-account erasure; blank individual limit restores the default. Disabling revokes sessions. Provider settings support `mock`, `openai`, `deepseek`, `anthropic`, and `gemini`. For live providers, enter a supported model ID and server-side API key, save, then test the saved connection. The connection button is disabled while fields have unsaved changes. Switching to a live provider requires a new key; omitting a key preserves the same live provider's existing key. Keys are never returned and use AES-GCM. Live connection tests can incur cost, have separate throttling, and do not consume analysis quotas.

### Local test AI and preview API

Select **Local test provider** (`mock`), enable generation, and save. Its fixed model is `local-example-v1`, and it needs no API key. Switching to mock clears the previously stored key. In a reflection review, select **Generate a local example report without sending my responses externally**. The browser submits `generate_report:true, local_example:true`; this explicit local path never contacts the network, needs no external-sharing consent, and consumes no daily generation quota, even if saved provider settings change meanwhile. Educational storage/processing consent still applies. Examples are clearly labeled, use your real deterministic scores/nulls, and include appropriate conflict caution. Their wording demonstrates report structure and is not personalized AI insight. Insufficient submissions keep their deterministic insufficient results.

The **Preview example response** control works even when a live provider is configured or inactive. It calls this admin-only, trusted-Origin API:

```text
POST /api/v1/admin/provider/preview
Authorization: Bearer <administrator access token>
Content-Type: application/json
Origin: <allowed browser origin>
```

```json
{"module":"self","locale":"en"}
```

Use module `self`, `compatibility`, or `conflict`; locale defaults to `fa`. Omit answers/context to use explicitly marked synthetic sample values, or supply your own typed values to preserve nulls/omissions. The response contains `provider:"mock"`, `is_example:true`, `external_request:false`, actual `scores`, a `kind:"example"` report, sample-use flags, and schema/prompt metadata. No external prompt/key lookup, analysis record, or generation quota is involved; only sanitized preview audit metadata is recorded. This endpoint does not call the live connection-test API.

Direct API callers who request a report without `local_example:true` use the normal consent/quota workflow, even when mock is the saved provider. Mock itself still makes no network calls and reports zero provider tokens. Activity counts include saved mock successes; preview requests create audit records only. See [API/local contracts](03-architecture.md#local-examples-and-preview).

Model access, JSON support, availability, and costs depend on the provider account. Check the relevant primary documentation: [OpenAI](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create), [DeepSeek](https://api-docs.deepseek.com/api/create-chat-completion/), [Anthropic](https://platform.claude.com/docs/en/build-with-claude/structured-outputs), [Gemini](https://ai.google.dev/api/generate-content). No live provider was tested here.

Limits controls global/default-user admitted generation attempts; per-user overrides apply. Provider retries may make multiple paid HTTP requests inside one attempt. Audit displays sanitized administrative actions without keys/answers. Ordinary accounts, including Google-only accounts, can be erased through the admin API with administrator password and exact target email; see [API contracts](03-architecture.md#http-api). Administrators are protected from those edits/erasure.

## Backup and updates

Native backup requires PostgreSQL client tools on PATH; Bash native backup also uses Python 3 to parse the connection URL without exposing it in command arguments.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/backup.ps1
# For containers:
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/backup.ps1 -Compose
```

```bash
bash scripts/backup.sh native
bash scripts/backup.sh compose
```

Backups use unique timestamped custom-format files under ignored `.local/backups/`. They are **not encrypted**. Protect storage and preserve master/signing configuration separately. Scripts validate native dump listings; test restoration into a fresh separate database before relying on a recovery plan. Never restore over live data merely to test a backup. No backup scheduler or report retention period is assumed.

Before updating: inspect changes, back up, test a separate restore, review migration compatibility, install locked dependencies, run checks, rebuild/restart, and verify readiness. Applied migrations and questionnaire wording are immutable. Old binaries may not work with a newer schema; prefer reviewed forward fixes. Publishing source to GitHub does not deploy the application; production rollout needs separate runtime acceptance.

## Troubleshooting

| Symptom | Diagnosis and action |
| --- | --- |
| Startup configuration error | Check named setting, mandatory secrets, literal env syntax, URL, cookie policy; direct Go commands need explicit env loading |
| Database unavailable/readiness 503 | Check pg_isready, URL/role/password/TLS, or Compose database health; do not print the secret DSN |
| Browser 5173 unavailable | Start Vite natively or frontend Compose service; confirm configured WEB_PORT and port conflicts |
| API connection/proxy error | Start API on configured target; Vite API_PROXY_TARGET and backend listener must agree |
| Origin 403 | Align exact browser scheme/hostname/port with CORS_ORIGINS; CLI mutations also need an allowed Origin |
| Profile required | Complete eligible age, gender, and name in Account |
| Strict JSON 400/415/413 | Send actual DTO fields, application/json, integer answers/null; body limit is 64 KiB |
| Draft revision conflict | Another save/tab changed it; preserve pending changes and reopen latest draft before saving |
| Unknown/null scores | Too few numeric answers; unknowns/omissions are intentionally not imputed |
| Similarity withheld | Need independent origin, partner sharing consent, and three sufficient shared dimensions |
| Daily limit 429 | Admitted AI attempt limit reached; choose deterministic scoring or wait for next UTC day |
| AI unavailable/invalid response/timeout | Check configured active provider/model/key/access; safe error code preserves scores; never paste keys/provider response bodies into diagnostics |
| Provider key unreadable | Master key changed or stored key corrupted; restore authorized key material or replace credential through admin settings |
| Auth 401 after password/disable/logout/replay | Sign in again; concurrent refresh must use the coordinated client |
| Temporary network/server error while refreshing | The error is shown without clearing the browser session; retry when the service is reachable. A denied refresh (401) clears the session and requires login |
| Google account_link_required | Use existing account method; implicit email linking is intentionally unavailable |
| Migration/catalog drift | Restore applied source or add reviewed new version/migration; never clear checksum records to force startup |
| Data seems missing after restart | Verify database URL/Compose project and persistent volume; do not delete volume |
| Default account absent | An account of that role already exists or automatic defaults were disabled; existing credentials are preserved. Use the configured initial account or explicit seed for a missing admin |

Health: `/api/v1/health/live` and `/api/v1/health/ready`. Logs contain route/status/request ID/latency, with no ordinary request body/credential dump. Review startup error text before sharing diagnostics. Docker/native Linux/live OAuth/provider runtime limits remain in [Development](04-development.md#verification-results).

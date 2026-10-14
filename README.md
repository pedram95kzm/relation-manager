# Relation-Manager

A React and Go application for educational self-reflection, relationship answer similarity, and conflict reflection. Includes English/Persian interfaces, light/dark themes, private saved drafts and results, optional AI narratives, and an administrator workspace. All project documentation is in English.

Original questionnaires are **not validated clinical tests**. Scores describe responses; similarity does not predict relationship success. AI narratives have additional limitations and require separate consent to share responses with the configured provider.

## Run locally

Requirements: Go 1.27+, Node.js 22.12+ (Node 24 tested), and PostgreSQL. Native commands read process environment; the supplied scripts load `.env` safely without executing its contents.

From the repository root in PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/setup-env.ps1
notepad .env
```

Set `DATABASE_URL` to your own PostgreSQL database. The setup script generates signing/encryption secrets and preserves an existing configuration. It does not install PostgreSQL or create a native database.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/run-backend.ps1
```

In a second terminal:

```powershell
Set-Location frontend
npm ci
npm run dev
```

Open [Relation-Manager](http://localhost:5173). Vite forwards `/api` to the API at `127.0.0.1:8080`. Create an account, complete the age/gender profile, and begin a reflection. No AI key is required for deterministic results.

Development/test startup creates these accounts only when the corresponding role has no account:

| Role | Email | Password |
| --- | --- | --- |
| Admin | `admin@relations.local` | `RelationsAdmin2026!` |
| User | `user@relations.local` | `RelationsUser2026!` |

Existing accounts are preserved. Override with `ADMIN_EMAIL`/`ADMIN_PASSWORD` and `USER_EMAIL`/`USER_PASSWORD`, or set `SEED_DEFAULT_ACCOUNTS=false`. Production disables automatic defaults and rejects these public passwords in configuration. Replace demo passwords before deploying a development database.

In **Administration → AI provider**, select **Local test provider**, save with generation enabled, and choose a local example in the questionnaire review. Example reports need no key, send nothing externally, and consume no generation quota. **Preview example response** also works independently of saved/live provider settings through `POST /api/v1/admin/provider/preview`.

For containers, prepare `.env` and run `docker compose up --build --wait` from the root. The frontend is published at localhost:5173; PostgreSQL and the API remain private. Docker runtime was unavailable on the implementation host; CI includes container smoke and browser checks.

For Linux commands, Google OAuth, first-admin initialization, AI providers, backup, and troubleshooting, read the [user guide](docs/02-user-guide.md).

## Included workflows

- Password accounts, rotating HttpOnly refresh sessions, Google callback UI, profiles, password changes, logout-all, and account deletion.
- Three original bilingual questionnaire wizards with explicit unknown answers, autosaved/resumable drafts, review and consent, deterministic scores, similarity coverage, accessible tables, history filters, deletion, and print/PDF export.
- Optional OpenAI, DeepSeek, Anthropic, and Gemini narratives with encrypted provider keys, six prompts, strict validation, bounded retries, and atomic UTC quotas; local example mode and a no-network preview API.
- Administrator analytics, user disable/limits/reauthenticated erasure, provider configuration/testing, daily limits, and sanitized audit records.
- Full-stack Compose, setup/backup/check scripts, and GitHub Actions verification.

## Documentation and verification

Start with the [documentation index](docs/README.md). Run backend tests with a dedicated `TEST_DATABASE_URL` whose database name contains `test`; otherwise database checks are skipped. Frontend commands are `npm test`, `npm run build`, and `npm run test:e2e` against a running stack.

The October 14 review passed real PostgreSQL backend tests, native builds, seven frontend tests, the production build, all four browser scenarios in one run, and an npm audit with zero reported vulnerabilities. See the [verification ledger](docs/04-development.md#verification-results) for exact results and external limits, and [remaining decisions](docs/05-decisions.md#remaining-work-and-open-questions) for release acceptance and optional extensions. Live Google/paid AI calls, Docker execution, native Linux runtime, and production TLS deployment require independent verification.

Repository: [pedram95kzm/relation-manager](https://github.com/pedram95kzm/relation-manager), using the `main` branch. **License remains undecided.** Keep `.env`, local clusters, dumps, logs, and personal reports out of Git.

# Documentation

The complete published documentation set consists of this index and five documents. Documentation is English-only; the application and questionnaires support English and Persian.

| Document | Purpose |
| --- | --- |
| [Overview](01-overview.md) | Delivered workflows, scope, interpretation, and limitations |
| [User guide](02-user-guide.md) | Windows/Linux setup, configuration, daily use, administration, backup, and troubleshooting |
| [Architecture](03-architecture.md) | Components, API contracts, persistence, authentication, scoring, AI, and security |
| [Development](04-development.md) | Builds, tests, integration/browser checks, CI, migration rules, and verification results |
| [Decisions](05-decisions.md) | Invariants, tradeoffs, risks, and outstanding operational decisions |

Reviewed against source on **2026-10-14**. The final local review corrected session refresh failure handling and passed PostgreSQL-backed backend checks, seven frontend tests, the production build, and all four browser scenarios in one run. Live provider accounts, Docker, and native Linux runtime remain unverified locally. The [verification ledger](04-development.md#verification-results) records executed checks and remaining limits; [Decisions](05-decisions.md#remaining-work-and-open-questions) records outstanding release decisions and optional extensions.

Start with the [user guide](02-user-guide.md) to run the application. Developers should also read [architecture](03-architecture.md) and [decisions](05-decisions.md). The [root README](../README.md) is the repository landing page.

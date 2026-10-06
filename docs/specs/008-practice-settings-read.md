# Practice settings aggregate read

Implement authenticated `GET /practices/settings` matching the current frontend
envelope: `{ success, data: { practice, settings, procedure_types, locations,
is_admin } }`. Practice is the explicit six-field projection; settings, procedures
and locations use explicit DTOs. Procedure and location lists exclude soft-deleted
rows and sort by `sort_order`. All active practice members may read; `is_admin` is
true only for owner/admin. Existing centralized middleware enforces membership.

## Schema prerequisite

Read-only Neon inspection on 2026-10-06 confirmed `available_weekdays` is absent
from both `practice_settings` and `practice_locations`. Add Goose migration 022,
equivalent to the frontend migration: non-null `SMALLINT[]`, default weekdays
1–6, with values constrained to 0–6 and at most seven entries.

Before commit, migration 022 was applied to the isolated Neon database. Goose first
recorded migration 021, whose `CREATE TABLE IF NOT EXISTS subscription` was a no-op
because the Better Auth table already existed. Metadata verification confirmed both
weekday columns, defaults, constraints, and migration records 021–022.

## Next.js compatibility

The response was checked against the current settings route, settings page queries,
and `SettingsClientProps` types. Its envelope and field names match; PostgreSQL
`SMALLINT[]` values serialize as the expected JavaScript `number[]`; nullable JSON
values remain null; and soft-delete filtering matches the settings page's current
queries and the correction documented in `API.md`.

## Tests

One focused integration test uses disposable local PostgreSQL and applies migration
022 there. It checks exact envelope fields, owner/staff `is_admin`, ordering,
soft-delete filtering, weekday defaults, and missing-settings failure. Run normal
unit/integration/vet checks. Leave changes uncommitted for review.

## Verification results

- `make test` passed.
- `make test-integration` passed against disposable local PostgreSQL; the cluster
  was removed afterward.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Migration 022 was applied to the isolated Neon database and verified through
  schema metadata. No Supabase data was accessed.

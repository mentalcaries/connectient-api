# Practice providers

Implement authenticated practice-scoped provider endpoints:

- `GET /practices/providers` for active practice members.
- `POST /practices/providers` for owner/admin.
- `PATCH /practices/providers/:id` for owner/admin.
- `PUT /practices/providers/:id/main` for owner/admin.
- `DELETE /practices/providers/:id` for owner/admin.

Responses project Provider fields plus `is_main`; callers never supply a practice
ID. Names are trimmed and required on create. Empty title becomes null and omitted
or empty specialty becomes `General`. PATCH is allowlisted and partial; supplied
names must remain nonempty, supplied empty title clears it, and supplied empty or
null specialty becomes `General`.

Provider and link creation are atomic. Setting main locks the practice, verifies
the tenant link, and atomically switches the main flag. The main provider cannot be
removed. DELETE removes only a non-main `practice_provider` link and returns 204;
the Provider row remains for historical appointment references.

## Ownership decision and integrity

Providers are single-practice-owned. Read-only Neon inspection found no shared
providers, duplicate links, or practices with multiple main providers. Migration
025 enforces one practice link per provider and one main provider per practice.

## Tests

Use unit tests for create/patch parsing. Use disposable local PostgreSQL for list
scoping, atomic creation, main switching, owner/admin authorization, staff denial,
tenant-scoped edits, main-removal protection, missing-link responses, and unlinking
without deleting Provider. Apply migration 025 to isolated Neon after verification.

## Verification results

- The contracts and normalization behavior were checked against the current Next.js
  provider actions and provider consumers.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL, including an injected link-failure rollback.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Migration 025 was applied to isolated Neon. Metadata confirms single-practice
  ownership, one-main-per-practice, and Goose version 25.
- No Supabase data was accessed or changed.

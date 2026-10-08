# Practice locations

Implement authenticated practice-scoped location endpoints:

- `GET /practices/locations` for active practice members.
- `POST /practices/locations` for owner/admin.
- `PATCH /practices/locations/:id` for owner/admin.

GET returns `{ success: true, data: Location[] }`, excludes soft-deleted rows,
includes inactive rows, and sorts by `sort_order` ascending. POST requires a
non-empty string name, accepts an optional string address, creates an active row
with database-default weekdays, and returns the row in a 201 envelope. Omitted or
empty create addresses become null, matching the current Next.js route.

POST locks the practice row and calculates the next sort order in one transaction
so concurrent API requests cannot allocate the same maximum. PATCH recognizes
only `name`, `address`, `is_active`, `sort_order`, `deleted_at`, and
`available_weekdays`. It supports nullable address/deletion time, strictly typed
values, normalized weekdays, and a tenant-scoped update. Unknown-only requests are
400 and cross-practice/missing IDs are 404.

## Schema correction

The current Next.js route and TypeScript types allow null location addresses, while
the Go/Neon schema has `address NOT NULL`. Migration 023 drops that constraint.

## Tests

Use small unit tests for create/patch payload parsing. Use disposable local
PostgreSQL for list filtering/order, creation defaults/order, owner/admin mutation,
staff denial, nullable updates, soft deletion, and tenant scoping. Apply migration
023 to isolated Neon after local verification, then verify schema metadata.

## Verification results

- `make test` and `make test-integration` passed; integration tests used disposable
  local PostgreSQL and the cluster was removed afterward.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Migration 023 was applied to isolated Neon. Metadata confirms
  `practice_locations.address` is nullable and Goose version 23 is applied.
- No Supabase data was accessed or changed.

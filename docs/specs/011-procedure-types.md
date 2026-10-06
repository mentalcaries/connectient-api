# Procedure types

Implement authenticated practice-scoped procedure type endpoints:

- `GET /practices/procedure-types` for active practice members.
- `POST /practices/procedure-types` for owner/admin.
- `PATCH /practices/procedure-types/:id` for owner/admin.
- `DELETE /practices/procedure-types/:id` for owner/admin.

GET returns `{ success: true, data: ProcedureType[] }`, excludes soft-deleted rows,
includes inactive rows, and sorts by `sort_order`. POST requires non-empty string
`name` and `value`, accepts optional boolean `is_primary`, and creates an active,
non-default row after the current maximum sort order. Creation locks the practice,
clears an existing primary when requested, and inserts in one transaction.

PATCH recognizes only `name`, `is_active`, and `sort_order`. Default procedure names
cannot be changed, but active state and sort order remain editable. DELETE is a soft
delete and rejects default procedures. Detail mutations are tenant scoped; missing
or cross-practice IDs return 404. Duplicate values return 409, including values on
soft-deleted rows.

## Database integrity

Migration 024 adds unique constraints for `(practice_id, value)` and one active,
non-deleted primary procedure per practice. Read-only inspection found no duplicate
value groups or practices with multiple current primaries in isolated Neon.

## Tests

Use unit tests for create/patch parsing. Use disposable local PostgreSQL for list
filtering/order, transactional creation, primary replacement, duplicate conflict,
owner/admin mutation, staff denial, default protections, soft deletion, and tenant
scoping. Apply migration 024 to isolated Neon after local verification.

## Verification results

- `make test` and `make test-integration` passed; integration tests used disposable
  local PostgreSQL and the cluster was removed afterward.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Migration 024 was applied to isolated Neon. Metadata confirms the practice/value
  unique constraint, current-primary partial unique index, and Goose version 24.
- No Supabase data was accessed or changed.

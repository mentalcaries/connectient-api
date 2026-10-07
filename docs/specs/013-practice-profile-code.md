# Practice profile and code

Implement:

- `GET /practices/profile` for active practice members.
- `PATCH /practices/profile` multipart full-form update for owner/admin.
- `PATCH /practices/practice-code` JSON update for owner/admin.

Profile GET returns `{ practice: PracticeDTO }` with explicit portal fields. Profile
PATCH preserves the current full-form semantics: required name, city, email, and
practice code; omitted optional text fields become empty strings; omitted
`has_multiple_providers` becomes false. Name/city require at least four characters,
email and website are validated, and practice code uses lowercase letters, digits,
and hyphens with length 4–30. Reserved/taken codes return 409.

## Logo storage

Production uses R2; Supabase Storage is retired. Go uses R2 for new logos with keys
`{practice_id}/logo_{timestamp}`. JPEG, PNG, and WebP are accepted after submitted
and detected content-type validation, with a 5 MiB maximum. Upload occurs before the
database update; a failed database update deletes the new object. The old owned R2
object is removed only after the database points at the replacement. Removal clears
the database first, then deletes the old owned object. URLs outside the configured
R2 public origin remain readable but are never interpreted as owned deletion keys.

Required Go configuration: `R2_ACCOUNT_ID`, `R2_BUCKET_NAME`,
`R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, and `R2_PUBLIC_URL`.

## Tests

Use unit tests for validation, MIME checks, and owned-key extraction. Use disposable
local PostgreSQL plus a fake object store for profile/code persistence, upload order
and cleanup, removal, authorization, duplicate/reserved code behavior, and tenant
scoping. No schema migration is expected.

## Verification results

- Multipart fields, validation, full-form reset behavior, and responses were checked
  against the current Next.js profile/code routes and settings consumers.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL and a fake object store.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon and external R2/Supabase services were not used by
  tests.

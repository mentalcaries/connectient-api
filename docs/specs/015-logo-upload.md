# Dedicated practice logo upload

Implement owner/admin `POST /upload/logo` and `DELETE /upload/logo` using R2.

POST accepts multipart `file`, verifies JPEG/PNG/WebP content, enforces 5 MiB,
uploads to `{practice_id}/logo_{timestamp}`, updates `practices.logo`, then removes
the previous owned R2 object. If the database update fails, the new object is
deleted. DELETE clears the database logo first and then removes the old owned R2
object. Non-R2 or cross-practice URLs are never used as deletion keys.

Responses match the current frontend: POST `{ url }`, DELETE `{ success: true }`.
No migration is required. Integration tests use disposable PostgreSQL and fake R2.

## Verification results

- Multipart fields and responses were checked against the current Next.js R2 logo
  routes.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL and fake R2, including injected database failure cleanup.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon and external storage were not changed.

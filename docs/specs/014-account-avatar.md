# Account and avatar

Implement authenticated `GET /account`, `PATCH /account`, `POST /upload/avatar`,
and `DELETE /upload/avatar`.

Account GET returns the caller's application profile and read-only identity email
and role. PATCH requires trimmed nonempty first/last names, always writes
`avatar_url` (omission clears it, preserving the current route), and optionally
writes nullable mobile phone and WhatsApp notification preference. It updates only
the JWT identity's own application row.

Avatar upload uses the fixed R2 key `{practice_id}/avatars/{user_id}`, accepts a
multipart `file`, verifies JPEG/PNG/WebP content, enforces 5 MiB, and returns
`{ url }` without updating the database. DELETE removes that fixed key and returns
`{ success: true }`. Account PATCH accepts only null or the caller's exact owned R2
avatar URL, correcting the current arbitrary-URL assignment risk.

No schema migration is required. Unit tests cover payload and ownership validation;
disposable PostgreSQL plus fake storage covers own-row updates, duplicate mobile,
tenant identity isolation, upload bounds/type, and fixed-key deletion.

## Verification results

- Contracts and two-step avatar behavior were checked against the current Next.js
  account page, account PATCH route, and avatar routes.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL and fake R2 storage.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon and external storage were not changed.

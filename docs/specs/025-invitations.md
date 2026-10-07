# Team invitations

Implement owner/admin invitation management and public validation:

- `GET/POST /users/invites`
- `DELETE /users/invites/:id`
- `POST /users/invites/:id/resend`
- `GET /invite/validate?token=`

Invitation creation validates and normalizes names/email, rejects an existing
practice membership, atomically locks seat allocation, and upserts a fresh UUID
token with seven-day expiry. Pending unexpired invites occupy seats. Lists never
return tokens. Delete and resend are practice scoped; resend rotates the token.
Email delivery uses an injectable provider and does not claim confirmed delivery.

Migration 029 adds the practice/email uniqueness required by upsert and concurrent
creation. Public validation returns only the documented invite/practice projection,
uses private/no-store, and rejects unknown, expired, or accepted tokens.

## Verification results

- Contracts were checked against the current Next.js invite management and public
  validation routes.
- `make test` and `make test-integration` passed against disposable PostgreSQL and
  a fake invite notifier; `go vet -tags=integration ./...`, gofmt, and diff checks
  passed.
- Isolated Neon contained no invites or duplicate groups. Migration 029 was applied
  and verified; Goose is at 29 and the unique index is present.

# Invitation acceptance and identity adapter

Implement claims-authenticated `POST /invite/accept` with
`{ token, first_name, last_name, mobile_phone?, termsAgreed }`.

Acceptance validates pending/unexpired token, true terms agreement, and a
case-insensitive match between verified JWT email and invitation email. It locks
the invite and practice seat namespace, rejects any existing different-practice
membership, creates the single application membership, and consumes the invite in
one transaction. Terms time is server generated.

Better Auth display-name synchronization uses an `IdentityProfileService` adapter,
not direct writes to Better Auth tables. The HTTP implementation posts
`{ user_id, name }` to `AUTH_PROFILE_SERVICE_URL` with bearer
`AUTH_PROFILE_SERVICE_TOKEN`. Acceptance is idempotent for the same identity,
practice, email, and accepted invite so a failed adapter call can be safely retried.
Adapter unavailability returns 503 `identity_sync_failed` after membership commit;
the retry performs no duplicate membership write and synchronizes the stored
membership name. The auth-service endpoint must authenticate the bearer service
token, update only the specified Better Auth user's display name, and treat the
same `{ user_id, name }` request idempotently.

No migration is required. Integration tests use disposable PostgreSQL and fake
identity adapters for atomic consumption, email mismatch, expiry, membership
conflict, concurrent acceptance, adapter retry, and tenant isolation.

## Verification results

- Request/response fields and errors were checked against the current Next.js
  invitation acceptance route.
- `make test` and `make test-integration` passed against disposable PostgreSQL;
  HTTP-adapter tests, `go vet -tags=integration ./...`, gofmt, and diff checks
  passed.
- Isolated Neon remains at Goose 29 and the invite uniqueness index is present;
  this slice required no migration.

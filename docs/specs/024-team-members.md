# Team members and seats

Implement owner/admin team endpoints:

- `GET /users`
- `PATCH /users/:id`
- `DELETE /users/:id`
- `GET /users/seats`

Lists exclude deleted memberships and return oldest first with `is_owner`. Mutations
are practice scoped, return 404 for missing targets, and cannot modify/delete the
owner or caller. Role is limited to admin/staff. Reactivation checks seats before
updating.

Seat limits remain essential 1, pro 3, practice 10. Usage counts active,
non-deleted memberships plus pending unexpired invitations. Missing/unknown plans
use the existing pro fallback. No migration is required. Integration tests use
disposable PostgreSQL for scoping, owner/self protection, zero-row behavior,
reactivation limits, invite occupancy, and plan limits.

## Verification results

- Fields, envelopes, role restrictions, and plan limits were checked against the
  current Next.js users and seat routes.
- `make test` and `make test-integration` passed against disposable PostgreSQL;
  `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon was not changed.

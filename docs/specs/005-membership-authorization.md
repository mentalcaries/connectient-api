# Enforce membership and route roles

Protected routes load current membership/practice state from PostgreSQL. Require
an active, non-deleted owner/admin/staff member of a non-suspended practice.
The connected-app list additionally requires owner role. Identity errors are 401,
denied membership is 403, and database lookup failures are 500.

Preserve claims-only `/register` and `/users/me` for onboarding and status checks.
JWT settings, subscription policy and public routes are outside this fix. The
query addition needs sqlc generation, not a database migration.

## Focused tests (simplified after user review)

- `TestMembershipAccess`: ordinary Go values test the allow/deny rules.
- `TestOwnerAccess`: exercises only the owner middleware with a supplied user.
- `TestAuthorizationQuery`: disposable PostgreSQL verifies membership loading,
  current suspension state and missing membership. Reuses the existing database
  fixture; no JWT signing, JWKS server or full-router setup.

```sh
go test ./internal/server -run '^Test(MembershipAccess|OwnerAccess)$' -v
TEST_DATABASE_URL='postgres://postgres@127.0.0.1:PORT/connectient_test?sslmode=disable' go test -tags=integration ./internal/server -run '^TestAuthorizationQuery$' -count=1 -v
```

These tests cover rules and query behaviour, not end-to-end JWT/middleware wiring,
route-policy placement or middleware error-status mapping. The broader integration
test was removed with approval to keep this change understandable. Leave changes
uncommitted for review.

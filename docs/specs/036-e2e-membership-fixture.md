# E2E membership fixture bridge

Provide the isolated E2E harness with a narrow way to remove a freshly-created
application membership when testing the unonboarded Better Auth flow.

- `DELETE /internal/test/memberships/:id` is available only when
  `E2E_TEST_MODE=true` and the shared `AUTH_PROFILE_SERVICE_TOKEN` is valid.
- The operation hard-deletes exactly one user ID and is idempotent. It exists only
  for test fixture setup and is unavailable in every normal runtime.
- Next.js continues to create the Better Auth identity/session, but no longer uses
  Supabase service-role credentials to mutate application membership.

No database migration is required.

## Verification results

- Unit coverage verifies the fixture is 404 outside E2E mode. Disposable
  PostgreSQL integration coverage verifies authenticated deletion removes the
  exact membership.
- SQL generation, `make test`, integration-tag vet, gofmt, diff checks, and the
  full disposable PostgreSQL integration suite pass. No migration is required;
  isolated Neon remains at Goose 34.

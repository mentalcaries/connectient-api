# Bind onboarding writes to one transaction

## Scope

Fix `POST /register` so its existing user, practice, membership link, settings,
default procedures and trial subscription writes all use the transaction already
started by the handler. Bind sqlc queries with `s.DBQuery.WithTx(tx)`; retain
existing request/response contracts, validation, commit and deferred rollback.

## Verification

Exercise the actual handler with disposable local PostgreSQL and existing schema
files. Verify the success response and committed related records; inject a failure
during procedure creation and during subscription creation and assert that all
earlier writes roll back. Also verify rollback on the reserved-code early return.
Tests require an explicit local `ONBOARDING_TEST_DATABASE_URL` pointing to database
`onboarding_transaction_test`; they never use application `DATABASE_URL`.
The suite is opt-in through the `integration` build tag. Setup helpers use
`t.Helper` and register resources with `t.Cleanup`; each subtest has an independent
timeout, and schema cleanup uses a fresh context even if the test times out.

Docker is unavailable on this machine, so use the installed PostgreSQL binaries
with a temporary cluster for verification. No Neon or Supabase data is involved.

Run against an explicit disposable local test database:

```sh
ONBOARDING_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:PORT/onboarding_transaction_test?sslmode=disable' go test -tags=integration ./internal/server -run '^TestRegistrationTransaction$' -count=1 -v
go vet -tags=integration ./internal/server
```

Verified: all four scenarios passed against a temporary local PostgreSQL cluster;
`go vet ./internal/server` and `git diff --check` passed. The cluster was stopped
and removed afterward. Tests create and clean up a unique schema per scenario.
No application migration is needed for this fix.

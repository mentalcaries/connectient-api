# Predictable test commands

`make test` runs ordinary Go tests without Docker or a database. Database tests
use the `integration` build tag and `make test-integration`, requiring an explicit
`TEST_DATABASE_URL` for a disposable local database named `connectient_test`.
Both database-service and handler integration tests use that same prerequisite;
remove the implicit Docker startup from the database package tests. Preserve
`make itest` as an alias. Missing test configuration fails clearly.

Correct the readiness response assertion, set Gin test mode for server tests,
and document commands in `docs/testing.md`. No application code or database
schema changes are part of this workflow fix. Keep changes uncommitted.

Verified: `make test` passes without Docker; `make test-integration` passes against
a temporary local PostgreSQL cluster, including both database and server packages.
Missing test configuration produces the intended setup message. `go vet
-tags=integration ./...` passes. The temporary cluster was removed afterward.

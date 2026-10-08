# Practice-scoped appointment detail and deletion

> Contract update: spec 030 removes hard deletion and introduces idempotent,
> state-preserving `POST /appointments/:id/cancel`. Detail tenant scoping remains
> applicable; historical DELETE behavior is no longer exposed.

Fix finding 4: `GET /appointments/:id` and `DELETE /appointments/:id` must match
both the path ID and the authenticated membership's practice ID in SQL. Bind those
parameters through regenerated sqlc queries. Return 403 for missing practice,
400 for invalid IDs, 404 for absent or other-practice appointments, and 500 for
other query errors. Keep existing successful DTOs and hard-delete behaviour.
Cancellation/soft-delete policy and centralized membership checks are separate.

No database migration is needed: only query predicates and handler calls change.
Test both endpoints against disposable local PostgreSQL: own-practice success,
other-practice denial, missing appointment, invalid ID, missing practice, database
failure. Assert failed requests leave records unchanged and deletion removes only
the authorized target. Use the existing opt-in local integration-test setup.

```sh
TEST_DATABASE_URL='postgres://postgres@127.0.0.1:PORT/connectient_test?sslmode=disable' go test -tags=integration ./internal/server -run '^TestAppointment' -count=1 -v
go vet -tags=integration ./internal/server
```

Verified: 12 detail/delete scenarios, the existing eight PATCH scenarios and
the PATCH route test passed on disposable local PostgreSQL. go vet and diff
checks passed. The temporary cluster was stopped and removed afterward.

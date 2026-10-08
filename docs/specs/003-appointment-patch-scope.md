# Appointment PATCH route and practice scope

> Contract update: spec 028 replaces the historical scheduling-body PATCH with
> the authoritative contact-only `{ email?, mobile_phone? }` contract. Scheduling
> now uses transactional `POST /appointments/:id/schedule`. The tenant scoping and
> zero-row behavior below remain applicable.

Fix finding 3: register `PATCH /appointments/:id` and pass the authenticated
membership's practice ID to the existing sqlc update query. Return 404 for
`pgx.ErrNoRows` (missing, other-practice or soft-deleted appointment), and retain
500 for other database errors. Membership without a practice returns 403 instead
of dereferencing a nil pointer. Existing update body and success DTO stay intact.
No schema migration, scheduling workflow or contact-edit contract changes.

Verify the actual route registration and exercise the handler/query against
disposable PostgreSQL for valid update, invalid ID/JSON, missing appointment,
other-practice appointment, deleted appointment, missing practice and database
failure. Confirm rejected requests leave persisted state unchanged. Reuse the
existing integration-test local database setup; no Neon/Supabase calls.

```sh
TEST_DATABASE_URL='postgres://postgres@127.0.0.1:PORT/connectient_test?sslmode=disable' go test -tags=integration ./internal/server -run '^TestAppointmentPatch' -count=1 -v
go vet -tags=integration ./internal/server
```

Verified: route registration and all eight handler/database scenarios passed
against a temporary local PostgreSQL cluster. `go vet -tags=integration
./internal/server` and `git diff --check` passed. The test cluster was stopped
and removed after verification.

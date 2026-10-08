# Patient detail history reads

Implement authenticated practice-scoped detail reads:

- `GET /patients/:id/appointments` returns `{ data: PatientAppointmentDTO[] }`,
  newest first and excluding soft-deleted appointments.
- `GET /patients/:id/registrations` returns
  `{ data: PatientRegistrationSummary | null }`, selecting the newest non-deleted
  registration only. This latest-only policy matches the current patient detail UI.

Both routes first verify that the patient belongs to the authenticated practice and
then scope child rows by both patient and practice. Appointment bearer tokens and
creator/scheduler audit IDs are not serialized. Registration tokens are not
serialized. Patient responses use `Cache-Control: private, no-store`.

Migration 027 reconciles the missing appointment `is_confirmed` column and state
constraint from the deployed Supabase migration. Existing scheduled rows are
backfilled as confirmed. Integration tests use disposable PostgreSQL for ordering,
soft-deletion, latest-registration selection, empty state, tenant isolation, and
token omission.

## Verification results

- The latest-only registration policy was explicitly selected; response fields
  match the current patient detail consumer while removing bearer tokens.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL; `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Isolated Neon preflight found 11 unscheduled rows and no prior confirmation
  column. Migration 027 was applied and verified: Goose is at 27, the column is
  non-null with default false, the constraint exists, and no invalid rows remain.

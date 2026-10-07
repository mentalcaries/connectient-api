# Appointment reads, availability, scheduling, and confirmation

Implement the current appointment HTTP contracts as one practice-scoped resource:

- `GET/POST /appointments`
- `PATCH /appointments/:id`
- `GET /appointments/confirmed`
- `GET /appointments/availability`
- `POST /appointments/:id/schedule`
- `POST /appointments/:id/confirm`

Public appointment requests are specified separately in spec 029.

All routes require active owner/admin/staff membership. Staff creation additionally
requires an active (not grace-only) subscription. Contact PATCH only updates trimmed
appointment email/phone fields. Reads are private/no-store and list newest first.

Availability emits 07:30–17:00 slots on a 15-minute cadence for one to seven days,
with overlap details for scheduled, non-cancelled provider appointments. Provider
and optional excluded appointment must belong to the caller's practice.

Creation and scheduling validate dates, times, duration, provider ownership,
location/weekday policy, and conflicts. Provider/date advisory locks serialize
conflict checks. Unacknowledged conflicts return 409 without mutation. Staff
creation atomically creates or updates the patient and creates the appointment;
duplicate new-patient contacts return `patient_exists`. Scheduling atomically locks
and updates an existing practice appointment. Confirmation uses one conditional
practice-scoped update so ineligible or concurrent confirmation returns 409.

Post-commit realtime/calendar hooks and appointment notifications use injectable
interfaces. Missing providers report `unavailable` where the wire contract exposes
delivery results and never attempt outbound traffic. Concrete delivery/calendar
providers remain part of their dedicated resource slices.

Migration 031 reconciles the legacy Go appointment table with staff creation by
allowing null request dates, appointment types, and legacy bearer tokens, and adds
the missing `created_by` user foreign key.

## Verification results

- Contracts, defaults, validation, conflict shape, patient matching, side-effect
  timing, and subscription behavior were checked against the current Next.js
  appointment routes and scheduling/availability services.
- `make test` and `make test-integration` passed against disposable PostgreSQL,
  including the pre-existing appointment scope/contact and patient-history suites;
  `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Isolated Neon preflight found 11 legacy appointments, no null tokens, and no
  orphaned `created_by` values. Migration 031 was applied and verified: Goose is at
  31, all three compatibility columns are nullable, and the audit foreign key is
  present.

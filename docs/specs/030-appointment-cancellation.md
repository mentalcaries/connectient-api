# Appointment cancellation

Replace historical appointment hard deletion with authenticated
`POST /appointments/:id/cancel` and body `{}`.

The handler derives practice from active membership, locks a non-deleted
practice-owned appointment, sets `is_cancelled` without deleting history, and
returns `{ appointment: AppointmentDTO }`. Repeated cancellation is idempotent and
returns the already-cancelled row. Missing, cross-practice, and soft-deleted rows
return 404. Scheduling explicitly rejects cancelled appointments so they cannot be
silently restored.

After commit, cancellation broadcasts the existing `DELETE` compatibility event
and invokes an injectable calendar deletion hook. Both are best effort. Per the
approved policy, cancellation sends no patient email or WhatsApp notification.

No migration is required.

## Verification results

- The route and response were checked against the cancellation proposal and the
  existing server action; the approved no-notification policy is explicit.
- `make test` and `make test-integration` passed against disposable PostgreSQL,
  including concurrent idempotency, tenant and soft-delete scope, one-time side
  effects, and cancelled-reschedule rejection. Integration-tag vet, gofmt, and
  diff checks passed.
- Isolated Neon remains at Goose 32; the existing `is_cancelled` column is present,
  so no migration was required.

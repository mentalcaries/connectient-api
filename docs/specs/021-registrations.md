# Authenticated patient registration core

Implement authenticated registration list, create, detail, soft-delete, and
compatibility link reads from `API.md`. Explicit send/resend operations and the
public token form workflow follow as separate registration slices.

## Duplicate and retention policy

A practice may have only one registration for the same exact patient name plus
email, or the same exact patient name plus phone. This applies to pending,
completed, expired, and soft-deleted records. Completed patients do not receive a
new form; expired links rotate the existing token through link/resend behavior.
Migration 028 enforces both contact variants so concurrent requests cannot bypass
the rule.

## Schema reconciliation

Migration 028 makes registration email nullable, adds nullable `patient_phone`,
requires at least one contact, and adds permanent partial unique indexes on
practice/name/email and practice/name/phone. These changes mirror the deployed
Supabase phone/contact migration and add the missing symmetric email constraint.

All authenticated responses are private/no-store, and tokens are never logged. The
following public-token slice must exclude deleted registrations, submit
transactionally, and reject suspended practices.

Creation supports optional notification requests through an injectable provider.
The record persists when delivery is unavailable or fails; successful fake-provider
delivery and `sent_at` tracking are covered here. Production email/WhatsApp wiring
is completed with the explicit delivery routes.

## Verification results

- Contracts were checked against the current Next.js list/create/detail/delete/link
  routes and registration consumers. The permanent duplicate policy was explicitly
  confirmed.
- `make test` and `make test-integration` passed against disposable PostgreSQL;
  `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Isolated Neon had no registrations or duplicate groups. Migration 028 was applied
  and verified; Goose is at 28, both contact columns are nullable, both unique
  indexes exist, and the contact-required constraint is present.

# Patients

Implement authenticated practice-scoped patient APIs:

- `GET /patients?q=` returns `{ patients: PatientSummaryWithDOB[] }`, newest first,
  limited to 20 search results or 500 unfiltered rows.
- `GET /patients/:id` returns `{ patient: PatientDTO }`.
- `PATCH /patients/:id` returns `{ patient: PatientDTO }`.

PATCH allowlists first/last names, email, mobile/home phone, date of birth, address
lines, city, email/WhatsApp consent, emergency contact name/phone, and notes. IDs,
practice ownership, and timestamps are server-owned even when present in the JSON.
The legacy `force` field is accepted but ignored; database uniqueness remains
authoritative. Empty nullable text/date values clear the field.

Duplicate email and mobile phone within a practice return 409 `duplicate_email` or
`duplicate_phone`. Migration 026 adds the same partial practice/contact unique
indexes evidenced in the deployed Supabase patient migration and missing from the
Go schema history. Cross-practice contacts remain valid.

Unit tests cover payload validation and server-owned field filtering. Integration
tests use disposable PostgreSQL for search bounds/order, tenant isolation,
allowlisted updates, clears, duplicate conflicts, and cross-practice contacts.

## Verification results

- Fields, envelopes, search sanitization, ordering, limits, and conflict codes were
  checked against the current Next.js patient routes and list/detail consumers.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL; `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Isolated Neon preflight found no duplicate contact groups. Migration 026 was
  applied and verified; Goose is at version 26 with both partial indexes present.
- The migration Make target was changed to suppress command echo so future runs do
  not print database credentials.

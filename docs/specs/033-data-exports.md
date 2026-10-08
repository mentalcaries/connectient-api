# Practice data exports

Implement owner/admin-only `GET /export/appointments` and
`GET /export/registrations`. Both routes derive practice and exporter identity
from authenticated membership, set private no-store headers, use static safe
attachment names, and record a durable audit row containing only practice, user,
export type, row count, and timestamp.

Appointment export is CSV with the existing columns, status derivation, date/time
formatting, location, notes, and generated-by footer. Every database string is
protected against spreadsheet formula injection before RFC 4180 escaping. Rows
are practice-scoped, non-deleted, newest first, and limited to 10,000 rows and a
50 MiB generated payload.

Registration export is a DOCX Open XML package generated with the Go standard
library. It includes completed, non-deleted, practice-scoped registrations and
their practice-scoped form JSON, newest first. Rendering is version-tolerant and
deterministic: known personal, additional, medical, dental, physiotherapy,
optometry, and signature sections use stable labels; unknown historical/custom
sections and nested values remain readable rather than being discarded. The
export is limited to 1,000 registrations and 25 MiB of source JSON.

Limit overflow returns 413 and no partial attachment or audit row. Any query,
render, or audit failure returns 500. Staff access is forbidden by the shared
owner/admin middleware.

Migration 034 adds `data_export_audit_log`.

## Verification results

- Owner/admin access was explicitly approved; staff access is forbidden.
- Output fields and formatting were checked against the current Next.js CSV and
  DOCX exporters. The DOCX renderer additionally preserves the newer specialty,
  nested TMJ, signature metadata, and unknown historical/custom sections.
- `make test` and the full disposable PostgreSQL integration suite passed. Golden
  tests cover CSV escaping/formula protection and valid DOCX package/text output;
  integration covers practice scope, status/deletion filters, no-store attachment
  headers, audit rows, and staff rejection. Integration-tag vet, gofmt, and diff
  checks passed.
- Isolated Neon preflight found Goose 33 and no audit table. Migration 034 was
  applied and verified: Goose is at 34 and the table, foreign keys, checks, and
  practice/time index are present.

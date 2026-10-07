# Public registration form

Implement public `GET` and `POST /registrations/form/:token`.

GET returns the allowlisted practice branding/category, form feature flags, theme,
and appointment-derived prefill. Unknown/deleted tokens return 404, completed forms
409, and expired links 410 while marking their status expired. Suspended, inactive,
or subscription-ineligible practices are hidden as 404.

POST accepts bounded JSON `{ form_version, form_data }`, validates the required
personal/additional fields, then locks the registration row and performs form-data
insert, patient find/create/enrichment, patient linking, and completion in one
transaction. Matching preserves the current rule: normalized email first, then
digits-only mobile, each combined with case-insensitive first/last names. Deleted,
completed, expired, suspended, and subscription-ineligible submissions cannot
write. Concurrent duplicate submissions deterministically produce 409.

Responses use private/no-store and tokens are never logged. No migration is
required because row locking plus registration status serializes submission.

## Verification results

- Bootstrap and submission contracts were checked against the current Next.js form
  loader, form component payload, and token route.
- `make test` and `make test-integration` passed against disposable PostgreSQL,
  including simultaneous submissions (one 201, one 409) and injected mid-transaction
  failure rollback.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon and production services were not changed.

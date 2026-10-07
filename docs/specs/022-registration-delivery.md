# Registration delivery

Implement authenticated registration delivery endpoints:

- `POST /registrations/:id/send-email`, optional `{ email }`.
- `POST /registrations/:id/send-whatsapp`, optional `{ phone }`.
- `POST /registrations/:id/resend`, always rotates the token and sends email.

All operations are practice scoped, reject completed/deleted registrations, and use
private/no-store responses. Existing-token send routes reject expired links with
410 rather than delivering an unusable link; staff must use resend, which rotates
the token for seven days and resets status to pending.

Contact overrides are saved before delivery and restored with a guarded conditional
update if delivery fails. Resend similarly restores the prior token, expiry, status,
and sent timestamp on delivery failure. Provider results are
`sent | simulated | unavailable | failed`; unavailable maps to 503 and failed to
500. Successful/simulated delivery updates `sent_at`, with tracking failure reported
separately. No schema migration is required.

## Verification results

- Contracts, fallback contacts, rollback behavior, and messages were checked
  against the current Next.js delivery routes. Failed provider delivery now
  consistently returns 500; unavailable configuration returns 503.
- `make test` and `make test-integration` passed against disposable PostgreSQL and
  fake delivery providers; `go vet -tags=integration ./...`, gofmt, and diff checks
  passed.
- No migration was required; Neon and production providers were not changed.
- Production Resend/Twilio implementations remain part of the notification-provider
  slice; an unwired provider safely reports unavailable rather than sending.

# Resend and Twilio notification providers

Implement one environment-configured provider for registration delivery, team
invites, appointment confirmation, and public new-request staff notifications.
The provider loads trusted practice, provider, location, and eligible staff data
from PostgreSQL and calls Resend/Twilio over bounded HTTP clients.

Safety is enforced before configuration or network access:

- `E2E_TEST_MODE=true` returns `simulated`.
- `OUTBOUND_MESSAGES_DISABLED=true`, `SKIP_EMAIL=true`, and
  `SKIP_WHATSAPP=true` return `unavailable` for the applicable channel.
- Missing credentials, sender values, templates, recipients, or required Twilio
  variables return `unavailable`.
- Provider HTTP failures return `failed` plus an error where the calling contract
  distinguishes failures.

Resend uses `RESEND_API_KEY`, `FROM_EMAIL` (default
`noreply@connectient.app`), and `EMAIL_SEND_FROM_DOMAIN`. Twilio uses the existing
account SID, auth token, WhatsApp sender, and content-template environment names.
Only active, non-deleted, opted-in staff with a phone receive new-request WhatsApp
messages. Successful patient deliveries are recorded best effort in
`notification_log`; log failure does not misreport provider delivery.

No migration is required.

## Verification results

- Provider environment names, template variables, recipient rules, subjects,
  text/HTML content, reply-to behavior, and logging were checked against the
  current Next.js Resend/Twilio helpers.
- `make test` and `make test-integration` passed against disposable PostgreSQL.
  Tests verify safety across every notifier interface with zero network calls,
  Resend/Twilio wire authentication and payloads, eligible-staff filtering, and
  successful-delivery logging. Integration-tag vet, gofmt, and diff checks passed.
- Isolated Neon remains at Goose 32; `notification_log` and all staff-filter fields
  are present, so no migration was required.

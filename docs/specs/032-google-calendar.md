# Google Calendar connection and one-way synchronization

Implement owner-only connected-app list, Google OAuth initiation, public callback,
and disconnect routes while preserving the popup postMessage contract. OAuth state
is random, stored server-side for ten minutes, mirrored in an HttpOnly SameSite=Lax
cookie, and consumed once. The scope remains exactly
`calendar.app.created email profile`.

Tokens use AES-256-GCM in the existing `iv:authTag:ciphertext` base64 format.
Refresh, calendar creation, and event operations use bounded HTTP clients. Every
practice writes only to its stored app-created `Connectient Appointments`
secondary calendar; the primary calendar is never targeted.

Scheduling stores the browser-supplied IANA timezone in
`appointments.scheduled_timezone`. Legacy rows default to the current backfill
meaning, `America/Port_of_Spain`. Event date-times remain offset-free wall-clock
values with the IANA zone supplied separately.

Create/update/cancel hooks maintain unique appointment/provider event mappings and
recover missing events or app calendars by recreating only in the app-owned
calendar. Disconnect transactionally deletes the connection and local mappings.
Every successful connect or reconnect starts best-effort backfill for future,
confirmed, scheduled, non-cancelled appointments only.
Connecting a different Google account transactionally clears old mappings and the
old app-calendar ID before backfill; reconnecting the same account preserves valid
mappings and suppresses duplicate events.

`E2E_TEST_MODE=true` or `SKIP_CALENDAR=true` prevents every Google network call.

Migration 033 adds the appointment timezone, app calendar ID, connection/mapping
uniqueness, and single-use OAuth state table.

## Verification results

- The OAuth paths, popup messages, scope, token format, app-owned-calendar rule,
  event fields, and recovery behavior were checked against the current Next.js
  integration. The prior timezone behavior was confirmed to be browser-derived for
  normal scheduling, UTC when omitted, and fixed Trinidad time during backfill.
- `make test` and the full disposable PostgreSQL integration suite passed. Tests
  cover single-use owner-bound OAuth state, encrypted credentials, restricted
  scope, deterministic timezone payloads, confirmed-only backfill, duplicate
  suppression, create/update/cancel mappings, disconnect cleanup, and zero Google
  calls in E2E mode. Integration-tag vet, gofmt, and diff checks passed.
- Isolated Neon preflight found Goose 32, no duplicate connections or mappings,
  and no legacy scheduled appointments. Migration 033 was applied and verified:
  Goose is at 33 and the timezone/calendar columns, OAuth state table, and both
  unique constraints are present.

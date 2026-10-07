# Appointment Realtime invalidation

Implement post-commit Supabase Realtime Broadcast over the supported HTTP API.
Messages use public topic `practice:{practice_id}`, event `INSERT`, `UPDATE`, or
`DELETE`, and the minimal payload `{ id, practice_id }`; no patient or appointment
details leave the API. Cancellation retains its compatibility `DELETE` event.

The provider uses `SUPABASE_URL` (falling back to the current
`NEXT_PUBLIC_SUPABASE_URL`) and `SUPABASE_SECRET_KEY` (falling back to the legacy
`SUPABASE_SERVICE_ROLE_KEY`) with a bounded HTTP client. `E2E_TEST_MODE=true` or
`SKIP_REALTIME=true` suppresses all network calls. Delivery is best effort after
the database commit and never repeats notification or calendar side effects.

Do not install a database appointment broadcast trigger alongside this publisher.
Before production enablement, inspect and remove/disable any legacy
`appointments_changes` or `handle_appointments_changes_v2` trigger, or set
`SKIP_REALTIME=true` until that cleanup is complete, to avoid duplicate frontend
refetches. The isolated Neon database has no non-internal appointment trigger and
no `realtime` schema.

No migration is required.

## Verification results

- The topic, event names, public-channel mode, and minimal payload were checked
  against the current Next.js publisher and subscriber. The current Supabase
  documentation confirms the topic/event REST endpoint used here.
- `make test` and the full disposable PostgreSQL integration suite passed. Wire
  tests assert method/path/auth headers and exactly two payload fields; safety
  tests prove E2E and skip flags make zero network calls. Provider/configuration
  failures and invalid events are also covered. Integration-tag vet, gofmt, and
  diff checks passed.
- Isolated Neon remains at Goose 34. It has no non-internal appointment trigger
  and no `realtime` schema, so no migration was required there. Production trigger
  inspection/cleanup remains an explicit deployment prerequisite.

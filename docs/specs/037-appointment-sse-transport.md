# Appointment SSE transport

Replace the temporary Supabase Realtime Broadcast provider with authenticated,
Go-hosted Server-Sent Events while preserving the appointment event abstraction and
minimal invalidation contract.

- `GET /appointments/events` requires active application membership and derives
  the subscription practice from that membership.
- One in-memory hub fans events out by practice for the current single-replica Go
  deployment. The hub uses bounded, coalescing subscriber queues so slow clients
  never block committed appointment commands.
- Events retain names `INSERT`, `UPDATE`, and `DELETE` and data
  `{ id, practice_id }`; no patient or appointment details are streamed.
- The stream sends an immediate connection comment, 15-second heartbeats, disables
  proxy buffering, and closes after five minutes so browser reconnection obtains a
  fresh JWT and rechecks membership.
- Calendar synchronization remains in the existing appointment event fanout. SSE
  delivery is best-effort after commit and does not duplicate notification or
  calendar effects.
- This transport supports exactly one active Go replica. Horizontal scaling must
  replace the in-memory hub with PostgreSQL, Redis, or another shared fanout while
  retaining the same interface.

No database migration is required.

## Verification

- Hub tests cover practice isolation, bounded coalescing, minimal payloads, and
  event validation.
- The SSE handler test proves headers and delivery continue beyond a configured
  server `WriteTimeout`.
- Go unit tests, race detection, integration-tag vet, and the full disposable
  PostgreSQL integration suite pass.

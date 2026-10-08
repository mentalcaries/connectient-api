# Appointment Realtime invalidation

> Superseded by spec 037. Supabase Broadcast was removed after the caller cutover
> in favor of authenticated Go-hosted SSE for the current single-replica deployment.

This historical Supabase Broadcast transport has been removed. Its replacement
retains the `INSERT`, `UPDATE`, and `DELETE` event names and minimal
`{ id, practice_id }` invalidation payload but delivers them through the
authenticated Go SSE endpoint described in spec 037. No Supabase publisher
configuration or skip flag remains in the Go runtime.

Do not install a database appointment broadcast trigger. The isolated Neon
database has no non-internal appointment trigger and no `realtime` schema.

No migration is required.

The replacement requires no database migration. Production trigger
inspection/cleanup remains an explicit deployment prerequisite.

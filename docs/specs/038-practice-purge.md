# Practice purge

Make cleanup of disposable practices reliable without requiring callers to know the
full application table graph.

## Contract

- Practice-owned foreign keys cascade from `practices` through users, appointments,
  patients, registrations, configuration, integrations, and join tables.
- Second-level registration form data and appointment calendar mappings cascade
  with their parent records.
- `SELECT purge_practice(<practice UUID>)` is the canonical complete cleanup. It
  deletes the Better Auth Stripe subscription whose text `referenceId` matches the
  practice, deletes the practice and all cascading records, then deletes providers
  that became unlinked.
- The function returns `true` when the practice existed and was purged, or `false`
  without changing data when the practice did not exist.
- No HTTP route exposes hard practice deletion. The function is intended for
  controlled database maintenance and isolated test cleanup.
- Function execution is revoked from `PUBLIC`; only the migration owner or a role
  explicitly granted access can invoke it.

## Safety and verification

- Provider IDs are captured before deleting their `practice_provider` links.
- Providers are deleted only when no remaining practice link exists.
- The operation is atomic because PostgreSQL functions execute within the caller's
  transaction.
- Integration coverage creates a complete practice graph, purges it with one call,
  verifies every owned record and provider/subscription orphan is gone, and verifies
  another practice is untouched.

## Verification results

- The full migration chain applies through Goose 36 and migration 036 rolls back to
  Goose 35 against disposable PostgreSQL.
- The complete Go integration suite passes against disposable PostgreSQL.

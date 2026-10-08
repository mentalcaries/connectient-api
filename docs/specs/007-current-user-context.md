# Current-user context and subscription policy

## Contract

Add claims-only `GET /me/context`. A valid JWT always receives HTTP 200 when the
database is available, including identities without application membership and
members whose access is revoked. Protected domain routes continue returning 403.

Response:

```json
{
  "identity": { "id": "uuid", "email": "user@example.com" },
  "membership": null,
  "practice": null,
  "subscription": null,
  "permissions": [],
  "onboarding_required": true
}
```

For an onboarded identity, membership contains ID, practice ID, role, names,
avatar, active/deleted state and `access_revoked`; practice contains the explicit
portal profile fields from `API.md`; subscription contains computed effective
status and RFC3339/null dates matching `src/lib/subscription.ts`.

Permission strings are stable API values:

- `bookings:access`, `registrations:access`, `calendar:access` when subscription
  is active or in its 30-day grace period.
- `settings:manage` and `team:manage` for owner/admin members while subscription
  access is active or in grace.
- `billing:manage` for active owner/admin members even when subscription is
  missing or expired, so recovery remains possible.
- `connected_apps:manage` for active owners with calendar subscription access.
- Revoked/deleted/inactive members and suspended practices receive no permissions.

Approved policy: an `active` subscription past `periodEnd` is effectively expired;
missing subscription blocks normal admin features; grace lasts 30 days; recovery
access survives subscription expiry but not membership/practice suspension.

## Compatibility correction

Keep `GET /users/me`. Return null for absent subscription dates rather than year-one
timestamps. Return 404 only for missing application user and 500 for database
failures. No schema migration is required; sqlc receives an explicit context query.

## Tests

Use small table-driven unit tests for subscription computation and permissions.
Use one focused PostgreSQL integration test for onboarded/unonboarded context,
nullable subscription dates and query failure handling. Do not test JWT signing
again. Run `make test`, local `make test-integration`, and go vet. Leave changes
uncommitted for review.

## Verification results

- `make test` passed, including subscription and permission tables.
- `make test-integration` passed against disposable local PostgreSQL, covering
  unonboarded, missing-subscription, active, suspended, nullable-date and database
  failure behavior. The cluster was removed afterward.
- `go vet -tags=integration ./...`, gofmt and diff checks passed.
- No Neon, Supabase, identity-provider or outbound-provider calls were made.

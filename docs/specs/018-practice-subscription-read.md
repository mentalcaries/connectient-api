# Practice subscription read

Implement authenticated `GET /practices/subscription` returning
`{ subscription: SubscriptionDTO }` for the caller's practice.

The DTO reuses the computed subscription policy used by `/me/context`: effective
status, plan, trial/expiry/grace/cancellation dates, remaining trial days, active,
expired and grace flags, feature access flags, and banner flags. Missing
subscriptions return the computed `none` state rather than an error so billing
recovery remains available. Raw Stripe identifiers and subscription rows are never
serialized.

The endpoint is scoped exclusively from the authenticated membership; it does not
accept a practice identifier. No schema migration is required. Integration tests
use disposable PostgreSQL for own-practice isolation, active/expired/grace and
missing-subscription responses.

## Verification results

- Computed fields and missing-subscription behavior were checked against the
  current Next.js `getSubscriptionStatus` consumer contract.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL, including own-practice isolation and sensitive-field assertions.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon and production data were not changed.

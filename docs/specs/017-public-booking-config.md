# Public booking configuration

Implement public practice reads:

- `GET /public/practices/:code/booking-config`
- `GET /public/practices/:code/procedure-types`
- `GET /public/practices/:code/locations`

All routes require an active, non-suspended practice whose subscription permits
booking (active/trialing or within the approved 30-day grace period). Unknown or
unavailable practices return 404 to avoid exposing private account state.

Booking config returns allowlisted public practice contact/branding fields,
theme/location settings, public providers when multiple-provider mode is enabled,
active non-deleted procedures, and active non-deleted locations with nonempty
weekday availability. Individual routes preserve the current Next.js envelopes.
Public procedure reads correct the current soft-delete leak.

No user, subscription, token, invitation, or suspension detail is serialized. No
schema migration is required. Integration tests use disposable PostgreSQL for
active, grace, expired, missing-subscription, suspended, filtering, and output
shape behavior.

## Verification results

- Response envelopes, displayed fields, location behavior, and procedure ordering
  were checked against the current Next.js public routes and booking-page loaders.
- `make test` and `make test-integration` passed against disposable local
  PostgreSQL, including access-policy, filtering, ordering, and sensitive-field
  assertions.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No migration was required; Neon and production data were not changed.

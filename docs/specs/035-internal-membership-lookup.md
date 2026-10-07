# Internal membership lookup

Provide a narrow service-to-service lookup for Better Auth/Stripe callbacks that
run without a browser request and therefore cannot exchange a session cookie for a
user JWT.

- `POST /internal/auth/membership` accepts strict JSON `{ userId: UUID }`.
- It requires `Authorization: Bearer <AUTH_PROFILE_SERVICE_TOKEN>` using a
  constant-time comparison. A missing server token makes the endpoint unavailable.
- Success returns `{ membership: { practice_id, role } }` only for active,
  non-deleted memberships. Unknown or revoked users return 404.
- The endpoint exposes no profile, subscription, or patient data and performs no
  mutation. It exists only for Stripe customer metadata and billing-reference
  authorization in the Better Auth process.

No database migration is required.

## Verification results

- Unit coverage verifies missing service credentials and malformed UUIDs fail
  before database access. Disposable PostgreSQL integration coverage verifies the
  minimal owner response and that inactive memberships are hidden with 404.
- `make test`, integration-tag vet, gofmt, diff checks, and the full disposable
  PostgreSQL integration suite pass. No migration is required; isolated Neon
  remains at Goose 34.

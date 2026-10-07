# Onboarding completion, practice codes, and walkthrough progress

Implement the canonical claims-authenticated onboarding routes:

- `GET /onboarding/check-code?code=`
- `GET /onboarding/suggest-code?name=`
- `POST /onboarding/complete`
- `GET /onboarding/progress`
- `PATCH /onboarding/progress/:key`

Completion accepts the documented `termsAgreed` DTO, uses verified JWT identity
and email, and atomically creates the owner membership, practice, category defaults,
default procedures, and 30-day pro trial. Existing membership and taken/reserved
practice codes return 409. The response is
`{ success: true, practice_id, redirect_to: '/admin/dashboard' }`.

Code check and suggestion preserve the current sanitization, three-segment slug,
and numeric suffix behavior. They require claims authentication but no existing
membership. Historical `/register` and public `/register/*` aliases are removed so
onboarding has one claims-authenticated HTTP contract.

Walkthrough keys are exactly `setup`, `appointments`, `create-appointment`, and
`registrations`. Reads return every key with either null or its timestamp object.
Updates accept at least one true `seen`, `dismissed`, or `completed` flag, use
server timestamps, and atomically upsert by JWT user/key without clearing unrelated
timestamps.

Migration 030 creates `onboarding_progress` with `(user_id, walkthrough_key)`
uniqueness and a foreign key to `users`.

## Verification results

- Paths, DTO fields, response envelopes, key allowlist, slug behavior, and
  timestamp semantics were checked against the current Next.js onboarding routes.
- `make test` and `make test-integration` passed against disposable PostgreSQL;
  `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Isolated Neon preflight found Goose 29, no progress table, and the expected
  lowercase `users` table. Migration 030 was applied and verified: Goose is at 30,
  and the table, unique index, and foreign key are present.

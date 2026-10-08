# Onboarding completion, practice codes, and walkthrough progress

Implement the canonical claims-authenticated onboarding routes:

- `GET /onboarding/check-code?code=`
- `GET /onboarding/suggest-code?name=`
- `POST /onboarding/complete`
- `GET /onboarding/progress`
- `PATCH /onboarding/progress/:key`

Completion accepts the documented `termsAgreed` DTO, uses verified JWT identity
and email, and atomically creates the owner membership, practice, category defaults,
default procedures, one linked main provider, and 30-day pro trial. A solo
registrant is always the main provider. A group-practice registrant may identify
themself as a clinician or supply the first provider's details; manager-led signup
cannot complete without a main provider. Existing membership and taken/reserved
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

Provider creation and its `practice_provider` main link are part of the same
onboarding transaction. Creating a provider through the settings API also promotes
the first linked provider to main even if the caller omits `is_main`, preventing a
practice with providers but no main provider.

Migration 035 repairs existing single-provider practices that have no provider
links by creating a deterministic provider from the earliest active owner and
linking it as main. It deliberately skips multi-provider practices, practices with
an existing provider link, and practices without an active owner because those
cases cannot be inferred safely.

## Verification results

- Paths, DTO fields, response envelopes, key allowlist, slug behavior, and
  timestamp semantics were checked against the current Next.js onboarding routes.
- `make test` and `make test-integration` passed against disposable PostgreSQL;
  `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- Provider coverage verifies solo-owner creation, required manager-supplied provider
  details, rollback, automatic promotion of the first settings-created provider,
  and migration 035's guarded backfill/skip behavior.
- Isolated Neon preflight found Goose 29, no progress table, and the expected
  lowercase `users` table. Migration 030 was applied and verified: Goose is at 30,
  and the table, unique index, and foreign key are present.

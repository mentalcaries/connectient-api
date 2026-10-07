# Public appointment requests

Implement `POST /public/practices/:code/appointment-requests` with a narrow,
strictly validated DTO and a required `Idempotency-Key` header.

The route resolves practice identity and notification metadata from the URL code,
uses the same hidden-404 active/grace booking policy as public bootstrap reads, and
validates active procedure, provider ownership, location ownership, multi-location
requirements, and configured weekday availability. The body is limited to 16 KiB.

Idempotency is scoped by practice and key. The server stores a SHA-256 hash of the
fully normalized request and the successful JSON response. Reusing the key with
the exact request replays that response without creating or notifying again;
reusing it with any changed value returns 409. Shared parent contact details do not
merge children: a patient is linked only when both names and email or phone match.
If contact data belongs to another name, the request remains valid but is left
unlinked, preserving the current best-effort patient behavior.

Creation, optional patient linking, and idempotency completion are transactional.
Realtime broadcast and trusted staff email/WhatsApp notification are best-effort
post-commit hooks behind injectable services. No patient cancellation notification
is introduced.

Distributed rate limiting is a deployment responsibility at the edge/gateway,
keyed by client IP and this route. This gives one consistent limit across replicas
and rejects abuse before it reaches Go or PostgreSQL. The application still
enforces body-size and validation limits.

Migration 032 creates the durable idempotency table.

## Verification results

- The DTO, weekday rules, trusted metadata, and patient-linking behavior were
  checked against the current booking form, server action, and patient helper.
- `make test` and `make test-integration` passed against disposable PostgreSQL,
  including exact and changed replays, concurrent identical submissions,
  cross-practice resources, and shared guardian contacts; integration-tag vet,
  gofmt, and diff checks passed.
- Isolated Neon preflight found Goose 31 and no idempotency table. Migration 032
  was applied and verified: Goose is at 32 and the table, composite primary key,
  and practice foreign key are present.

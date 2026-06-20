# Connectient API

## Description

This is the backend (REST API) for Connectient — an appointment booking and patient management app for healthcare practices. This represents the final split and decoupling of the initial Next.js/Supabase architecture. The routes set up here are derived directly from the API routes initially set up in Next.js, and bypass the Supabase client directly to work with any PostgreSQL database — essentially allowing for portability of the application.

#### Stack for this configuration:

- **Framework:** Gin
- **Database driver:** pgx
- **Database provider:** Neon
- **Migrations:** Goose
- **Query generation:** sqlc

Auth is currently handled by Better Auth — this API validates JWTs using the JWKS endpoint exposed by the TypeScript (Next.js) server.

---

## Auth Flow

1. Client authenticates via Better Auth in the Next.js app (magic link, Google OAuth).
2. Next.js issues a short-lived JWT via the Better Auth `jwt` plugin.
3. Requests to this API include the JWT as a Bearer token (`Authorization: Bearer <token>`).
4. This API fetches the JWKS from `FRONTEND_BASE_URL/api/auth/jwks` and verifies the token signature.
5. Depending on the route's middleware tier, the token's claims are either used directly or used to look up the corresponding row in the `users` table.

There are three route tiers:

- **Public** — no token required.
- **Claims only** — token must be valid, but no `users` row is expected to exist yet (used during registration, before onboarding is complete).
- **Authenticated** — token must be valid AND a corresponding `users` row must exist. The hydrated user (id, role, practice_id, etc.) is attached to the Gin context for handlers to use.

---

## Environment Variables

```
DATABASE_URL=          # Neon Postgres connection string
FRONTEND_BASE_URL=     # Base URL of the Next.js app, used to fetch JWKS for JWT verification
PORT=                  # Port the server listens on
```

---

## Setup

```bash
go mod download

# Run migrations
goose -dir ./migrations postgres "$DATABASE_URL" up

# Generate sqlc code (after editing any .sql query files)
sqlc generate

# Run the server (hot reload via Air)
air
```

---

## Migrations

Migrations are managed with [Goose](https://github.com/pressly/goose) and live in `./migrations`. Each schema change should be its own migration file using the `-- +goose Up` / `-- +goose Down` format.

**Note:** the `subscription` table is owned and managed by Better Auth's Stripe plugin, not by this API. A Goose migration exists for it solely so sqlc can validate queries against its schema — do not write a real `Down` migration for it, as running `goose down` should never drop a table the auth layer depends on.

---

## sqlc

Query files live in `./queries` (one `.sql` file per domain, e.g. `users.sql`, `practices.sql`). Generated Go code should never be hand-edited — re-run `sqlc generate` after any query change.

---

## Routes

### Public

| Method | Route | Handler |
|---|---|---|
| GET | `/` | `handleReadiness` |
| GET | `/health` | `healthHandler` |
| POST | `/appointments` | `handlerAppointmentsCreate` |
| GET | `/register/suggest-code` | `handlerSuggestPracticeCode` |
| GET | `/register/check-code` | `handlerCheckCodeAvailability` |

### Claims Only (JWT verified, no DB hydration)

| Method | Route | Handler |
|---|---|---|
| POST | `/register` | `handlerNewRegistration` |
| GET | `/users/me` | `handlerGetCurrentUser` |

### Authenticated (JWT verified + DB-hydrated user)

| Method | Route | Handler |
|---|---|---|
| GET | `/appointments` | `handlerAppointmentsGetAll` |
| GET | `/appointments/:id` | `handlerGetAppointmentById` |
| PATCH | `/appointments` | `handlerAppointmentsUpdate` |
| DELETE | `/appointments/:id` | `handlerAppointmentsDelete` |
## MakeFile

Run build make command with tests
```bash
make all
```

Build the application
```bash
make build
```

Run the application
```bash
make run
```
Create DB container
```bash
make docker-run
```

Shutdown DB Container
```bash
make docker-down
```

DB Integrations Test:
```bash
make itest
```

Live reload the application:
```bash
make watch
```

Run the test suite:
```bash
make test
```

Clean up binary from the last build:
```bash
make clean
```

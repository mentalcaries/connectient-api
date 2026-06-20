# Connectient API

One Paragraph of project description goes here

## Description

Here is the backend (REST API) for Connectient. This represents the final split and decoupling of the initial Next.js/Supabase architecture. The routes set up here are derived directly from the API routes initially set up in Next.js, and bypasses the Supabase client directly to work with any PostgreSQL database - essentially allowing for portability of the application.


#### Stack for this configuration:

*Gin*
*pgx*
*Database Provider:* NeonDB

Auth currently handled by Better-Auth - this API validates JWTs using the JWK endpoint exposed by the TypeScript Server.

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

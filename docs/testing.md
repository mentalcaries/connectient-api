# Running tests

## Everyday tests

```sh
make test
```

Runs unit tests without Docker, PostgreSQL or application configuration. Gin debug
output is disabled. To see individual cases:

```sh
go test ./internal/server -v
```

## Database tests

Prerequisite: a running **local PostgreSQL** instance and an empty, disposable
database named `connectient_test`. Docker is not required or started by the tests.
For an existing local PostgreSQL installation, create the database once:

```sh
createdb -h 127.0.0.1 connectient_test
export TEST_DATABASE_URL="postgres://$(whoami)@127.0.0.1:5432/connectient_test?sslmode=disable"
make test-integration
```

Adjust the local username, password and port for your installation. Never use a
Neon or production URL. Tests require the explicit test variable rather than
falling back to application `DATABASE_URL`. The test role needs schema-creation
permission. Handler tests create unique schemas from the checked-in SQL and remove
them afterward; database-service tests check connection health and shutdown.

`make test-integration` runs unit and integration tests. `make itest` is an alias.
Without `TEST_DATABASE_URL`, the integration command fails with setup instructions.
No tests start the API or contact messaging/OAuth providers.

## Cleaning up a test practice

After migration 036 is applied, purge a disposable practice and all of its owned
records with one database call:

```sql
SELECT purge_practice('00000000-0000-0000-0000-000000000000');
```

The function also removes the matching subscription and providers left unlinked by
the cascade. It returns `true` when it deletes a practice and `false` when the UUID
does not exist. Execution is restricted to the migration owner unless it is granted
explicitly to another maintenance role.

For one integration test (after exporting the variable):

```sh
go test -tags=integration ./internal/server -run '^TestAuthorizationQuery$' -count=1 -v
```

## Static checks

```sh
go vet -tags=integration ./...
```

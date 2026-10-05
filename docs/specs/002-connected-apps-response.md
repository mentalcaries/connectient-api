# Connected-app list: token-free response

## Scope

Fix finding 2 in the API comparison: `GET /practices/connected-apps` currently
serializes stored OAuth token fields and maps the access token into the refresh
token field. Use the `API.md` section 4.8 response projection:
`{ provider, connected_account_email, connected }[]`.

Select only provider, connected account email and connection status in SQL;
regenerate sqlc output. Email remains present as JSON null when absent. An empty
list remains `[]`. Removing expiry from the response also removes the nullable
expiry dereference. Return immediately after a query error rather than appending
a success response. Preserve the current route, practice scope and error status;
authorization changes are a separate finding.

## Verification

Add isolated handler tests for the exact public JSON shape, connected/disconnected
states, null email, empty results and query failure. Check the practice query
argument and ensure the executed projection does not request token fields.
Run focused Go tests, go vet and formatting/diff checks. No live database or OAuth
provider calls are required; this changes the query/DTO, not the schema.

Verified: all four `TestConnectedAppsResponse` subtests pass; `go vet
./internal/server`, gofmt and `git diff --check` pass. sqlc regenerated only the
connected-app query output. Existing callers consuming `is_connected` must use
the documented `connected` field when adopting this response.

# Practice settings update

Implement authenticated `PATCH /practices/settings` for active owner/admin members.
The route returns `{ "success": true }` and updates practice/settings rows in one
transaction.

## Accepted fields

Practice: `specialty`, `has_multiple_providers`.

Settings: `dental_history_enabled`, `tmj_history_enabled`,
`multiple_locations_enabled`, `available_weekdays`, `custom_form_sections`,
`physiotherapy_history_enabled`, `optometry_history_enabled`, and `theme`.
`theme_colors` is recognized only when `theme` is present. Unknown fields are
ignored; a request containing no recognized fields is rejected.

Weekdays must be an array of integers from 0 through 6. They are deduplicated and
sorted; an empty array is valid. Boolean fields require JSON booleans. Specialty
accepts a string or null. Custom form sections accept any valid JSON, including
null, matching the current database column.

Themes are limited to the frontend's current preset IDs plus `match_logo`. When
supplied, `theme_colors` must be null or an object containing exactly the required
`start`, `mid`, `end`, `btn`, `dir`, and `accent` string values. Colours use six
digit hex notation and direction is one of the four directions in `src/lib/themes.ts`.
Omitted colours are stored as null, matching the current Next.js route.

## Failure behavior

Malformed JSON, invalid field types/values, and requests without recognized fields
return 400. Staff receive 403. Database failures return 500 and roll back both
practice and settings changes.

## Tests

Use table-driven unit tests for normalization and theme validation. Use disposable
local PostgreSQL to verify persistence, clearing nullable fields, authorization,
and transaction rollback. Do not use Neon or Supabase for test data.

The Next.js forwarding route remains responsible for calling `revalidatePath` after
a successful Go response; cache invalidation is not a backend API operation.

## Verification results

- The accepted payload and response were checked against the current settings UI.
- `make test` passed, including validation tables.
- `make test-integration` passed against disposable local PostgreSQL, including
  owner/admin access, staff denial, nullable clears, and injected-failure rollback.
- `go vet -tags=integration ./...`, gofmt, and diff checks passed.
- No Neon or Supabase test data was read or changed.

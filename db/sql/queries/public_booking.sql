-- name: GetPublicBookingPractice :one
SELECT
    p.id, p.name, p.logo, p.city, p.street_address, p.phone, p.email, p.website,
    p.practice_code, p.instagram, p.facebook, p.has_multiple_providers,
    p.practice_category, p.specialty, p.is_active, p.is_suspended,
    ps.multiple_locations_enabled, ps.available_weekdays, ps.theme, ps.theme_colors,
    s.status AS subscription_status, s.plan AS subscription_plan,
    s."trialEnd" AS subscription_trial_end,
    s."periodEnd" AS subscription_period_end,
    s."cancelAt" AS subscription_cancel_at
FROM practices p
JOIN practice_settings ps ON ps.practice_id = p.id
LEFT JOIN subscription s ON s."referenceId" = p.id::text
WHERE p.practice_code = sqlc.arg(practice_code);

-- name: GetPublicProcedureTypes :many
SELECT id, name, value, sort_order, is_primary
FROM procedure_types
WHERE practice_id = sqlc.arg(practice_id)
  AND is_active = TRUE
  AND deleted_at IS NULL
ORDER BY sort_order ASC;

-- name: GetPublicLocations :many
SELECT id, name, sort_order, available_weekdays
FROM practice_locations
WHERE practice_id = sqlc.arg(practice_id)
  AND is_active = TRUE
  AND deleted_at IS NULL
  AND cardinality(available_weekdays) > 0
ORDER BY sort_order ASC;

-- name: GetPublicProviders :many
SELECT p.id, p.first_name, p.last_name, p.title, p.specialty
FROM practice_provider pp
JOIN provider p ON p.id = pp.provider_id
WHERE pp.practice_id = sqlc.arg(practice_id)
ORDER BY pp.is_main DESC, p.created_at ASC, p.id ASC;

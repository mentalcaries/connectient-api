-- name: CreatePracticeSettings :one
INSERT INTO practice_settings (
    practice_id,
    dental_history_enabled,
    tmj_history_enabled,
    multiple_locations_enabled,
    custom_form_sections,
    physiotherapy_history_enabled,
    optometry_history_enabled
) VALUES (
    sqlc.arg(practice_id),
    sqlc.arg(dental_history_enabled),
    sqlc.arg(tmj_history_enabled),
    sqlc.arg(multiple_locations_enabled),
    sqlc.arg(custom_form_sections),
    sqlc.arg(physiotherapy_history_enabled),
    sqlc.arg(optometry_history_enabled)
)
RETURNING id;

-- name: UpdatePracticeSettings :exec
UPDATE practice_settings
SET
    dental_history_enabled        = COALESCE(sqlc.narg(dental_history_enabled), dental_history_enabled),
    tmj_history_enabled           = COALESCE(sqlc.narg(tmj_history_enabled), tmj_history_enabled),
    multiple_locations_enabled    = COALESCE(sqlc.narg(multiple_locations_enabled), multiple_locations_enabled),
    custom_form_sections          = COALESCE(sqlc.narg(custom_form_sections), custom_form_sections),
    physiotherapy_history_enabled = COALESCE(sqlc.narg(physiotherapy_history_enabled), physiotherapy_history_enabled),
    optometry_history_enabled     = COALESCE(sqlc.narg(optometry_history_enabled), optometry_history_enabled),
    theme                         = COALESCE(sqlc.narg(theme), theme),
    theme_colors                  = COALESCE(sqlc.narg(theme_colors), theme_colors),
    updated_at                    = NOW()
WHERE practice_id = sqlc.arg(practice_id);


-- name: GetPracticeSettings :one
SELECT * FROM practice_settings
WHERE practice_id = sqlc.arg(practice_id);
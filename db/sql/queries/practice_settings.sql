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

-- name: PatchPracticeSettings :execrows
UPDATE practice_settings
SET
    dental_history_enabled = CASE
        WHEN sqlc.arg(set_dental_history_enabled)::boolean THEN sqlc.arg(dental_history_enabled)::boolean
        ELSE dental_history_enabled
    END,
    tmj_history_enabled = CASE
        WHEN sqlc.arg(set_tmj_history_enabled)::boolean THEN sqlc.arg(tmj_history_enabled)::boolean
        ELSE tmj_history_enabled
    END,
    multiple_locations_enabled = CASE
        WHEN sqlc.arg(set_multiple_locations_enabled)::boolean THEN sqlc.arg(multiple_locations_enabled)::boolean
        ELSE multiple_locations_enabled
    END,
    available_weekdays = CASE
        WHEN sqlc.arg(set_available_weekdays)::boolean THEN sqlc.arg(available_weekdays)::smallint[]
        ELSE available_weekdays
    END,
    custom_form_sections = CASE
        WHEN sqlc.arg(set_custom_form_sections)::boolean THEN sqlc.narg(custom_form_sections)::jsonb
        ELSE custom_form_sections
    END,
    physiotherapy_history_enabled = CASE
        WHEN sqlc.arg(set_physiotherapy_history_enabled)::boolean THEN sqlc.arg(physiotherapy_history_enabled)::boolean
        ELSE physiotherapy_history_enabled
    END,
    optometry_history_enabled = CASE
        WHEN sqlc.arg(set_optometry_history_enabled)::boolean THEN sqlc.arg(optometry_history_enabled)::boolean
        ELSE optometry_history_enabled
    END,
    theme = CASE
        WHEN sqlc.arg(set_theme)::boolean THEN sqlc.arg(theme)::text
        ELSE theme
    END,
    theme_colors = CASE
        WHEN sqlc.arg(set_theme)::boolean THEN sqlc.narg(theme_colors)::jsonb
        ELSE theme_colors
    END,
    updated_at = NOW()
WHERE practice_id = sqlc.arg(practice_id);


-- name: GetPracticeSettings :one
SELECT * FROM practice_settings
WHERE practice_id = sqlc.arg(practice_id);

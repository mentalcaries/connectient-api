-- +goose Up
CREATE TABLE practice_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    practice_id UUID NOT NULL UNIQUE,
    dental_history_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    tmj_history_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    multiple_locations_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    optometry_history_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    physiotherapy_history_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    custom_form_sections JSONB,
    theme TEXT NOT NULL DEFAULT 'default',
    theme_colors JSONB,
    CONSTRAINT practice_settings_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id)
);

-- +goose Down
DROP TABLE practice_settings;

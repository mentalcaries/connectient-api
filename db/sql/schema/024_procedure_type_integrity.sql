-- +goose Up
ALTER TABLE procedure_types
ADD CONSTRAINT procedure_types_practice_value_unique
UNIQUE (practice_id, value);

CREATE UNIQUE INDEX procedure_types_one_current_primary
ON procedure_types (practice_id)
WHERE is_primary = TRUE AND deleted_at IS NULL;

-- +goose Down
DROP INDEX procedure_types_one_current_primary;

ALTER TABLE procedure_types
DROP CONSTRAINT procedure_types_practice_value_unique;

-- +goose Up
ALTER TABLE practice_locations
ALTER COLUMN address DROP NOT NULL;

-- +goose Down
UPDATE practice_locations
SET address = ''
WHERE address IS NULL;

ALTER TABLE practice_locations
ALTER COLUMN address SET NOT NULL;

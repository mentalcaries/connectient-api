-- +goose Up
ALTER TABLE practice_settings
ADD COLUMN available_weekdays SMALLINT[] NOT NULL
DEFAULT ARRAY[1, 2, 3, 4, 5, 6]::SMALLINT[];

ALTER TABLE practice_settings
ADD CONSTRAINT practice_settings_available_weekdays_valid
CHECK (
    available_weekdays <@ ARRAY[0, 1, 2, 3, 4, 5, 6]::SMALLINT[]
    AND cardinality(available_weekdays) <= 7
);

ALTER TABLE practice_locations
ADD COLUMN available_weekdays SMALLINT[] NOT NULL
DEFAULT ARRAY[1, 2, 3, 4, 5, 6]::SMALLINT[];

ALTER TABLE practice_locations
ADD CONSTRAINT practice_locations_available_weekdays_valid
CHECK (
    available_weekdays <@ ARRAY[0, 1, 2, 3, 4, 5, 6]::SMALLINT[]
    AND cardinality(available_weekdays) <= 7
);

-- +goose Down
ALTER TABLE practice_locations
DROP CONSTRAINT practice_locations_available_weekdays_valid,
DROP COLUMN available_weekdays;

ALTER TABLE practice_settings
DROP CONSTRAINT practice_settings_available_weekdays_valid,
DROP COLUMN available_weekdays;

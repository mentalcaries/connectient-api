-- +goose Up
ALTER TABLE practice_provider
ADD CONSTRAINT practice_provider_provider_unique UNIQUE (provider_id);

CREATE UNIQUE INDEX practice_provider_one_main
ON practice_provider (practice_id)
WHERE is_main = TRUE;

-- +goose Down
DROP INDEX practice_provider_one_main;

ALTER TABLE practice_provider
DROP CONSTRAINT practice_provider_provider_unique;

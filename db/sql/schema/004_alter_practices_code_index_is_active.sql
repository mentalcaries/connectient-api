-- +gooseUp
ALTER TABLE practices
ADD COLUMN is_active BOOLEAN NOT NULL
DEFAULT TRUE;

CREATE UNIQUE INDEX IF NOT EXISTS practice_code_unique_idx
ON practices(practice_code);

-- +goose Down
ALTER TABLE practices
DROP COLUMN is_active;
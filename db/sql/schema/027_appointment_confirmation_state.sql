-- +goose Up
ALTER TABLE appointments
ADD COLUMN IF NOT EXISTS is_confirmed BOOLEAN;

UPDATE appointments
SET is_confirmed = (is_scheduled IS TRUE)
WHERE is_confirmed IS NULL;

ALTER TABLE appointments
ALTER COLUMN is_confirmed SET DEFAULT FALSE,
ALTER COLUMN is_confirmed SET NOT NULL;

ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_confirmed_requires_schedule;

ALTER TABLE appointments
ADD CONSTRAINT appointments_confirmed_requires_schedule
CHECK (is_confirmed IS FALSE OR is_scheduled IS TRUE);

-- +goose Down
ALTER TABLE appointments
DROP CONSTRAINT IF EXISTS appointments_confirmed_requires_schedule,
DROP COLUMN IF EXISTS is_confirmed;

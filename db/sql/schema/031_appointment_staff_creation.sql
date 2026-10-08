-- +goose Up
ALTER TABLE appointments
    ALTER COLUMN requested_date DROP NOT NULL,
    ALTER COLUMN appointment_type DROP NOT NULL,
    ALTER COLUMN token DROP NOT NULL;

ALTER TABLE appointments
    ADD CONSTRAINT appointments_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES users(id);

-- +goose Down
ALTER TABLE appointments
    DROP CONSTRAINT IF EXISTS appointments_created_by_fkey;

ALTER TABLE appointments
    ALTER COLUMN requested_date SET NOT NULL,
    ALTER COLUMN appointment_type SET NOT NULL,
    ALTER COLUMN token SET NOT NULL;

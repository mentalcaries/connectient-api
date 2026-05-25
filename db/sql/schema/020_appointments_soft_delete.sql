-- +goose Up

ALTER TABLE appointments
ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;

-- +goose Down
ALTER TABLE appointments
DROP COLUMN deleted_at;
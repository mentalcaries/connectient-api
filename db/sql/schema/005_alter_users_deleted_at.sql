-- +gooseUp
ALTER TABLE users
ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;


-- +goose Down
ALTER TABLE users
DROP COLUMN deleted_at;
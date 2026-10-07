-- +goose Up
CREATE UNIQUE INDEX practice_invites_practice_email_idx
ON practice_invites (practice_id, email);

-- +goose Down
DROP INDEX practice_invites_practice_email_idx;

-- +goose Up
CREATE UNIQUE INDEX patients_practice_email_idx
ON patients (practice_id, email)
WHERE email IS NOT NULL;

CREATE UNIQUE INDEX patients_practice_mobile_idx
ON patients (practice_id, mobile_phone)
WHERE mobile_phone IS NOT NULL;

-- +goose Down
DROP INDEX patients_practice_mobile_idx;
DROP INDEX patients_practice_email_idx;

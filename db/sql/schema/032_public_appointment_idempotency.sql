-- +goose Up
CREATE TABLE public_appointment_request_idempotency (
    practice_id UUID NOT NULL REFERENCES practices(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    request_hash BYTEA NOT NULL,
    response_body BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (practice_id, idempotency_key),
    CHECK (char_length(idempotency_key) BETWEEN 8 AND 128)
);

-- +goose Down
DROP TABLE public_appointment_request_idempotency;

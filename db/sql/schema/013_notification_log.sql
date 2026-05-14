-- +goose Up
CREATE TABLE notification_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    practice_id UUID NOT NULL,
    channel TEXT NOT NULL CHECK (channel = ANY (ARRAY['email', 'whatsapp'])),
    notification_type TEXT NOT NULL,
    CONSTRAINT notification_log_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id)
);

-- +goose Down
DROP TABLE notification_log;

-- +goose Up
CREATE TABLE connected_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    practice_id UUID NOT NULL,
    provider TEXT NOT NULL,
    connected_account_email TEXT,
    access_token TEXT,
    refresh_token TEXT,
    token_expires_at TIMESTAMP WITH TIME ZONE,
    is_connected BOOLEAN NOT NULL DEFAULT TRUE,
    last_error TEXT,
    CONSTRAINT connected_apps_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id)
);

-- +goose Down
DROP TABLE connected_apps;

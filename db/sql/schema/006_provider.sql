-- +goose Up
CREATE TABLE provider (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    title TEXT,
    specialty TEXT NOT NULL DEFAULT 'general'
);

-- +goose Down
DROP TABLE provider;

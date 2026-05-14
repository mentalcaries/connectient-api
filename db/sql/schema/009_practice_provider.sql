-- +goose Up
CREATE TABLE practice_provider (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    practice_id UUID NOT NULL,
    provider_id UUID NOT NULL,
    is_main BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT practice_provider_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id),
    CONSTRAINT practice_provider_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES provider(id)
);

-- +goose Down
DROP TABLE practice_provider;

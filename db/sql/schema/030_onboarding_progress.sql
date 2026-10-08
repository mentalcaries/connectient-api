-- +goose Up
CREATE TABLE onboarding_progress (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    walkthrough_key TEXT NOT NULL,
    seen_at TIMESTAMPTZ,
    dismissed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, walkthrough_key)
);

-- +goose Down
DROP TABLE onboarding_progress;

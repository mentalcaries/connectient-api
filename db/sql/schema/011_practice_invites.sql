-- +goose Up
CREATE TABLE practice_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    practice_id UUID NOT NULL,
    email TEXT NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    org_role TEXT,
    role TEXT NOT NULL CHECK (role = ANY (ARRAY['admin', 'staff'])),
    invited_by UUID NOT NULL,
    token TEXT NOT NULL UNIQUE,
    token_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    accepted_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT practice_invites_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id),
    CONSTRAINT practice_invites_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES users(id)
);

-- +goose Down
DROP TABLE practice_invites;

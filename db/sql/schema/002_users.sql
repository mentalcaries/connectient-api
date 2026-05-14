-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    modified_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    mobile_phone TEXT UNIQUE,
    email VARCHAR,
    practice_id UUID,
    role TEXT DEFAULT 'staff' CHECK (role = ANY (ARRAY['owner', 'admin', 'staff'])),
    org_role TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    invited_by UUID,
    avatar_url TEXT,
    whatsapp_notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    terms_agreed_at TIMESTAMP WITH TIME ZONE,
    CONSTRAINT users_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id),
    CONSTRAINT users_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES users(id)
);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION update_modified_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.modified_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER users_modified_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_modified_at();

-- +goose Down
DROP TRIGGER IF EXISTS users_modified_at ON users;
DROP FUNCTION IF EXISTS update_modified_at();
DROP TABLE users;
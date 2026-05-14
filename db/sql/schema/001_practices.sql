-- +goose Up
CREATE TABLE practices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    modified_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    name TEXT NOT NULL,
    city TEXT NOT NULL,
    phone TEXT,
    email TEXT,
    practice_code TEXT NOT NULL UNIQUE,
    logo TEXT,
    street_address TEXT,
    facebook TEXT,
    instagram TEXT,
    website TEXT,
    has_multiple_providers BOOLEAN NOT NULL DEFAULT false,
    specialty TEXT,
    is_suspended BOOLEAN NOT NULL DEFAULT false,
    practice_category TEXT NOT NULL CHECK (practice_category = ANY (ARRAY['dental', 'medical', 'optometry', 'physiotherapy']))
);

-- +goose Down
DROP TABLE practices;
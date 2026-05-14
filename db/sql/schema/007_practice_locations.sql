-- +goose Up
CREATE TABLE practice_locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    practice_id UUID NOT NULL,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT practice_locations_practice_id_fkey FOREIGN KEY (practice_id) REFERENCES practices(id)
);

-- +goose Down
DROP TABLE practice_locations;

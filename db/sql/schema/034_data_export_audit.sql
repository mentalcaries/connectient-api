-- +goose Up
CREATE TABLE data_export_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    practice_id UUID NOT NULL REFERENCES practices(id) ON DELETE CASCADE,
    exported_by_user_id UUID NOT NULL REFERENCES users(id),
    export_type TEXT NOT NULL CHECK (export_type IN ('appointments', 'registrations')),
    row_count INTEGER NOT NULL CHECK (row_count >= 0),
    exported_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX data_export_audit_log_practice_exported_at_idx
ON data_export_audit_log (practice_id, exported_at DESC);

-- +goose Down
DROP TABLE data_export_audit_log;

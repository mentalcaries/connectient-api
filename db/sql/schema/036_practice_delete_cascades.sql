-- +goose Up
ALTER TABLE users
    DROP CONSTRAINT users_practice_id_fkey,
    ADD CONSTRAINT users_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE appointments
    DROP CONSTRAINT appointments_practice_id_fkey,
    ADD CONSTRAINT appointments_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE practice_locations
    DROP CONSTRAINT practice_locations_practice_id_fkey,
    ADD CONSTRAINT practice_locations_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE patients
    DROP CONSTRAINT patients_practice_id_fkey,
    ADD CONSTRAINT patients_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE practice_provider
    DROP CONSTRAINT practice_provider_practice_id_fkey,
    ADD CONSTRAINT practice_provider_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE practice_settings
    DROP CONSTRAINT practice_settings_practice_id_fkey,
    ADD CONSTRAINT practice_settings_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE practice_invites
    DROP CONSTRAINT practice_invites_practice_id_fkey,
    ADD CONSTRAINT practice_invites_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE notification_log
    DROP CONSTRAINT notification_log_practice_id_fkey,
    ADD CONSTRAINT notification_log_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE procedure_types
    DROP CONSTRAINT procedure_types_practice_id_fkey,
    ADD CONSTRAINT procedure_types_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE connected_apps
    DROP CONSTRAINT connected_apps_practice_id_fkey,
    ADD CONSTRAINT connected_apps_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE patient_registrations
    DROP CONSTRAINT patient_registrations_practice_id_fkey,
    ADD CONSTRAINT patient_registrations_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id) ON DELETE CASCADE;

ALTER TABLE patient_registration_data
    DROP CONSTRAINT patient_registration_data_registration_id_fkey,
    ADD CONSTRAINT patient_registration_data_registration_id_fkey
        FOREIGN KEY (registration_id) REFERENCES patient_registrations(id) ON DELETE CASCADE;

ALTER TABLE appointment_calendar_events
    DROP CONSTRAINT appointment_calendar_events_appointment_id_fkey,
    ADD CONSTRAINT appointment_calendar_events_appointment_id_fkey
        FOREIGN KEY (appointment_id) REFERENCES appointments(id) ON DELETE CASCADE;

CREATE INDEX users_practice_id_idx
    ON users (practice_id);
CREATE INDEX appointments_practice_id_idx
    ON appointments (practice_id);
CREATE INDEX practice_locations_practice_id_idx
    ON practice_locations (practice_id);
CREATE INDEX patients_practice_id_idx
    ON patients (practice_id);
CREATE INDEX practice_provider_practice_id_idx
    ON practice_provider (practice_id);
CREATE INDEX notification_log_practice_id_idx
    ON notification_log (practice_id);
CREATE INDEX procedure_types_practice_id_idx
    ON procedure_types (practice_id);
CREATE INDEX patient_registrations_practice_id_idx
    ON patient_registrations (practice_id);
CREATE INDEX patient_registration_data_registration_id_idx
    ON patient_registration_data (registration_id);
CREATE INDEX google_oauth_states_practice_id_idx
    ON google_oauth_states (practice_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION purge_practice(target_practice_id UUID)
RETURNS BOOLEAN AS $$
DECLARE
    linked_provider_ids UUID[];
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM practices
        WHERE id = target_practice_id
    ) THEN
        RETURN FALSE;
    END IF;

    SELECT COALESCE(array_agg(provider_id), ARRAY[]::UUID[])
    INTO linked_provider_ids
    FROM practice_provider
    WHERE practice_id = target_practice_id;

    DELETE FROM subscription
    WHERE "referenceId" = target_practice_id::TEXT;

    DELETE FROM practices
    WHERE id = target_practice_id;

    DELETE FROM provider AS candidate
    WHERE candidate.id = ANY(linked_provider_ids)
      AND NOT EXISTS (
          SELECT 1
          FROM practice_provider
          WHERE provider_id = candidate.id
      );

    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

REVOKE EXECUTE ON FUNCTION purge_practice(UUID) FROM PUBLIC;

-- +goose Down
DROP FUNCTION IF EXISTS purge_practice(UUID);

DROP INDEX IF EXISTS google_oauth_states_practice_id_idx;
DROP INDEX IF EXISTS patient_registration_data_registration_id_idx;
DROP INDEX IF EXISTS patient_registrations_practice_id_idx;
DROP INDEX IF EXISTS procedure_types_practice_id_idx;
DROP INDEX IF EXISTS notification_log_practice_id_idx;
DROP INDEX IF EXISTS practice_provider_practice_id_idx;
DROP INDEX IF EXISTS patients_practice_id_idx;
DROP INDEX IF EXISTS practice_locations_practice_id_idx;
DROP INDEX IF EXISTS appointments_practice_id_idx;
DROP INDEX IF EXISTS users_practice_id_idx;

ALTER TABLE appointment_calendar_events
    DROP CONSTRAINT appointment_calendar_events_appointment_id_fkey,
    ADD CONSTRAINT appointment_calendar_events_appointment_id_fkey
        FOREIGN KEY (appointment_id) REFERENCES appointments(id);

ALTER TABLE patient_registration_data
    DROP CONSTRAINT patient_registration_data_registration_id_fkey,
    ADD CONSTRAINT patient_registration_data_registration_id_fkey
        FOREIGN KEY (registration_id) REFERENCES patient_registrations(id);

ALTER TABLE patient_registrations
    DROP CONSTRAINT patient_registrations_practice_id_fkey,
    ADD CONSTRAINT patient_registrations_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE connected_apps
    DROP CONSTRAINT connected_apps_practice_id_fkey,
    ADD CONSTRAINT connected_apps_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE procedure_types
    DROP CONSTRAINT procedure_types_practice_id_fkey,
    ADD CONSTRAINT procedure_types_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE notification_log
    DROP CONSTRAINT notification_log_practice_id_fkey,
    ADD CONSTRAINT notification_log_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE practice_invites
    DROP CONSTRAINT practice_invites_practice_id_fkey,
    ADD CONSTRAINT practice_invites_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE practice_settings
    DROP CONSTRAINT practice_settings_practice_id_fkey,
    ADD CONSTRAINT practice_settings_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE practice_provider
    DROP CONSTRAINT practice_provider_practice_id_fkey,
    ADD CONSTRAINT practice_provider_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE patients
    DROP CONSTRAINT patients_practice_id_fkey,
    ADD CONSTRAINT patients_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE practice_locations
    DROP CONSTRAINT practice_locations_practice_id_fkey,
    ADD CONSTRAINT practice_locations_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE appointments
    DROP CONSTRAINT appointments_practice_id_fkey,
    ADD CONSTRAINT appointments_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

ALTER TABLE users
    DROP CONSTRAINT users_practice_id_fkey,
    ADD CONSTRAINT users_practice_id_fkey
        FOREIGN KEY (practice_id) REFERENCES practices(id);

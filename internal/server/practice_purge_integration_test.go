//go:build integration

package server

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type practicePurgeFixture struct {
	practiceID     uuid.UUID
	userID         uuid.UUID
	providerID     uuid.UUID
	patientID      uuid.UUID
	appointmentID  uuid.UUID
	registrationID uuid.UUID
}

func TestPurgePracticeDeletesCompleteOwnedGraph(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newPracticePurgeTestPool(t, ctx)

	purged := insertPracticePurgeFixture(t, ctx, pool, "purged")
	retained := insertPracticePurgeFixture(t, ctx, pool, "retained")

	var deleted bool
	if err := pool.QueryRow(ctx, `SELECT purge_practice($1)`, purged.practiceID).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("purge_practice returned false for an existing practice")
	}

	for _, table := range []string{
		"practices", "users", "practice_settings", "practice_locations", "patients",
		"practice_provider", "practice_invites", "notification_log", "procedure_types",
		"connected_apps", "patient_registrations", "public_appointment_request_idempotency",
		"google_oauth_states", "data_export_audit_log", "appointments",
	} {
		assertPracticePurgeCount(t, ctx, pool, table, purged.practiceID, 0)
		assertPracticePurgeCount(t, ctx, pool, table, retained.practiceID, 1)
	}

	for _, record := range []struct {
		table string
		id    uuid.UUID
	}{
		{"provider", purged.providerID},
		{"users", purged.userID},
		{"patients", purged.patientID},
		{"appointments", purged.appointmentID},
		{"patient_registrations", purged.registrationID},
	} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+record.table+" WHERE id = $1", record.id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("%s retained purged record %s", record.table, record.id)
		}
	}

	for _, table := range []string{"appointment_calendar_events", "patient_registration_data", "onboarding_progress"} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("%s has %d rows after purge, want retained fixture only", table, count)
		}
	}

	for _, referenceID := range []struct {
		id   uuid.UUID
		want int
	}{{purged.practiceID, 0}, {retained.practiceID, 1}} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM subscription WHERE "referenceId" = $1`, referenceID.id.String()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != referenceID.want {
			t.Errorf("subscription %s count = %d, want %d", referenceID.id, count, referenceID.want)
		}
	}

	if err := pool.QueryRow(ctx, `SELECT purge_practice($1)`, uuid.New()).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("purge_practice returned true for an unknown practice")
	}
	assertPracticePurgeCount(t, ctx, pool, "practices", retained.practiceID, 1)
}

func newPracticePurgeTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyTestSchemas(t, ctx, pool,
		"003_appointments.sql", "006_provider.sql", "007_practice_locations.sql", "008_patients.sql",
		"009_practice_provider.sql", "011_practice_invites.sql", "013_notification_log.sql",
		"015_patient_registrations.sql", "016_patient_registration_data.sql",
		"017_appointment_calendar_events.sql", "018_connected_apps.sql", "019_appointments_fkeys.sql",
		"020_appointments_soft_delete.sql", "022_available_weekdays.sql", "023_location_address_nullable.sql",
		"024_procedure_type_integrity.sql", "025_provider_ownership.sql", "026_patient_contact_uniqueness.sql",
		"027_appointment_confirmation_state.sql", "028_registration_contacts.sql",
		"029_practice_invite_email_unique.sql", "030_onboarding_progress.sql",
		"031_appointment_staff_creation.sql", "032_public_appointment_idempotency.sql",
		"033_google_calendar_sync.sql", "034_data_export_audit.sql",
		"035_backfill_single_provider_owners.sql", "036_practice_delete_cascades.sql",
	)
	return pool
}

func insertPracticePurgeFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) practicePurgeFixture {
	t.Helper()
	fixture := practicePurgeFixture{
		practiceID:     uuid.New(),
		userID:         uuid.New(),
		providerID:     uuid.New(),
		patientID:      uuid.New(),
		appointmentID:  uuid.New(),
		registrationID: uuid.New(),
	}
	locationID := uuid.New()

	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, $2, 'Test City', $3, 'medical')`, []any{fixture.practiceID, "Purge " + suffix, "purge-" + suffix}},
		{`INSERT INTO users (id, first_name, last_name, email, mobile_phone, practice_id, role)
			VALUES ($1, 'Test', 'Owner', $2, $3, $4, 'owner')`, []any{fixture.userID, suffix + "@example.test", "+1555" + fixture.userID.String()[:8], fixture.practiceID}},
		{`INSERT INTO provider (id, first_name, last_name, specialty)
			VALUES ($1, 'Test', 'Provider', 'General')`, []any{fixture.providerID}},
		{`INSERT INTO practice_provider (practice_id, provider_id, is_main) VALUES ($1, $2, TRUE)`, []any{fixture.practiceID, fixture.providerID}},
		{`INSERT INTO practice_settings (practice_id) VALUES ($1)`, []any{fixture.practiceID}},
		{`INSERT INTO practice_locations (id, practice_id, name, address) VALUES ($1, $2, 'Main', '1 Test Street')`, []any{locationID, fixture.practiceID}},
		{`INSERT INTO patients (id, practice_id, first_name, last_name, email)
			VALUES ($1, $2, 'Test', 'Patient', $3)`, []any{fixture.patientID, fixture.practiceID, "patient-" + suffix + "@example.test"}},
		{`INSERT INTO appointments
			(id, first_name, last_name, email, mobile_phone, requested_date, requested_time,
			 appointment_type, practice_id, provider_id, location_id, patient_id, created_by, scheduled_by, token)
			VALUES ($1, 'Test', 'Patient', $2, '+15550000000', CURRENT_DATE, 'flexible',
			'consultation', $3, $4, $5, $6, $7, $7, $8)`, []any{
			fixture.appointmentID, "appointment-" + suffix + "@example.test", fixture.practiceID,
			fixture.providerID, locationID, fixture.patientID, fixture.userID, "appointment-" + suffix,
		}},
		{`INSERT INTO appointment_calendar_events (appointment_id, external_event_id)
			VALUES ($1, $2)`, []any{fixture.appointmentID, "calendar-" + suffix}},
		{`INSERT INTO patient_registrations
			(id, practice_id, appointment_id, patient_id, patient_name, patient_email, token,
			 token_expires_at, sent_by_user_id)
			VALUES ($1, $2, $3, $4, 'Test Patient', $5, $6, NOW() + INTERVAL '1 day', $7)`, []any{
			fixture.registrationID, fixture.practiceID, fixture.appointmentID, fixture.patientID,
			"registration-" + suffix + "@example.test", "registration-" + suffix, fixture.userID,
		}},
		{`INSERT INTO patient_registration_data (registration_id, form_version, form_data)
			VALUES ($1, '1', '{}')`, []any{fixture.registrationID}},
		{`INSERT INTO practice_invites
			(practice_id, email, first_name, last_name, role, invited_by, token, token_expires_at)
			VALUES ($1, $2, 'Invited', 'User', 'staff', $3, $4, NOW() + INTERVAL '1 day')`, []any{
			fixture.practiceID, "invite-" + suffix + "@example.test", fixture.userID, "invite-" + suffix,
		}},
		{`INSERT INTO notification_log (practice_id, channel, notification_type)
			VALUES ($1, 'email', 'test')`, []any{fixture.practiceID}},
		{`INSERT INTO procedure_types (practice_id, name, value, is_primary)
			VALUES ($1, 'Consultation', $2, TRUE)`, []any{fixture.practiceID, "consultation-" + suffix}},
		{`INSERT INTO connected_apps (practice_id, provider, connected_account_email)
			VALUES ($1, 'google_calendar', $2)`, []any{fixture.practiceID, "calendar-" + suffix + "@example.test"}},
		{`INSERT INTO subscription (plan, "referenceId", status)
			VALUES ('pro', $1, 'trialing')`, []any{fixture.practiceID.String()}},
		{`INSERT INTO onboarding_progress (user_id, walkthrough_key, seen_at)
			VALUES ($1, 'setup', NOW())`, []any{fixture.userID}},
		{`INSERT INTO public_appointment_request_idempotency
			(practice_id, idempotency_key, request_hash)
			VALUES ($1, $2, decode('00', 'hex'))`, []any{fixture.practiceID, "idempotency-" + suffix}},
		{`INSERT INTO google_oauth_states (state, practice_id, user_id, expires_at)
			VALUES ($1, $2, $3, NOW() + INTERVAL '10 minutes')`, []any{"oauth-" + suffix, fixture.practiceID, fixture.userID}},
		{`INSERT INTO data_export_audit_log (practice_id, exported_by_user_id, export_type, row_count)
			VALUES ($1, $2, 'appointments', 1)`, []any{fixture.practiceID, fixture.userID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("insert %s fixture: %v\n%s", suffix, err, statement.sql)
		}
	}
	return fixture
}

func assertPracticePurgeCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, practiceID uuid.UUID, want int) {
	t.Helper()
	column := "practice_id"
	if table == "practices" {
		column = "id"
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+column+" = $1", practiceID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Errorf("%s practice %s count = %d, want %d", table, practiceID, count, want)
	}
}

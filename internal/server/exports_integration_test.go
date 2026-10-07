//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestDataExportsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newDataExportTestPool(t, ctx)
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	adminID, staffID, otherUserID := uuid.New(), uuid.New(), uuid.New()
	for _, fixture := range []struct {
		id   uuid.UUID
		code string
	}{{practiceID, "export-practice"}, {otherPracticeID, "other-export"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, 'Export Practice', 'Test City', $2, 'dental')`, fixture.id, fixture.code); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range []struct {
		id, practice uuid.UUID
		role         string
		email        string
	}{{adminID, practiceID, "admin", "admin@example.test"}, {staffID, practiceID, "staff", "staff@example.test"}, {otherUserID, otherPracticeID, "owner", "other@example.test"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role, is_active)
			VALUES ($1, 'Export', 'User', $4, $2, $3, TRUE)`, fixture.id, fixture.practice, fixture.role, fixture.email); err != nil {
			t.Fatal(err)
		}
	}
	locationID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practice_locations (id, practice_id, name, is_active)
		VALUES ($1, $2, 'Main Office', TRUE)`, locationID, practiceID); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		id, practice uuid.UUID
		first, email string
		deleted      bool
	}{{uuid.New(), practiceID, "Own", "own@example.test", false}, {uuid.New(), practiceID, "Deleted", "deleted@example.test", true}, {uuid.New(), otherPracticeID, "Other", "other-patient@example.test", false}} {
		if _, err := pool.Exec(ctx, `INSERT INTO appointments
			(id, practice_id, first_name, last_name, email, mobile_phone, location_id, created_at, deleted_at)
			VALUES ($1, $2, $3, 'Patient', $4, '+15550000000', $5, NOW(), CASE WHEN $6 THEN NOW() ELSE NULL END)`,
			fixture.id, fixture.practice, fixture.first, fixture.email, nullableExportLocation(fixture.practice == practiceID, locationID), fixture.deleted); err != nil {
			t.Fatal(err)
		}
	}
	registrationIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	registrationFixtures := []struct {
		practice uuid.UUID
		status   string
		deleted  bool
		name     string
		user     uuid.UUID
	}{
		{practiceID, "completed", false, "Own Registration", adminID},
		{practiceID, "completed", true, "Deleted Registration", adminID},
		{practiceID, "pending", false, "Pending Registration", adminID},
		{otherPracticeID, "completed", false, "Other Registration", otherUserID},
	}
	for index, fixture := range registrationFixtures {
		if _, err := pool.Exec(ctx, `INSERT INTO patient_registrations
			(id, practice_id, patient_name, patient_email, token, token_expires_at, status,
			 sent_by_user_id, completed_at, deleted_at)
			VALUES ($1, $2, $3, $4, $5, NOW() + INTERVAL '7 days', $6, $7,
			 CASE WHEN $6='completed' THEN NOW() ELSE NULL END, CASE WHEN $8 THEN NOW() ELSE NULL END)`,
			registrationIDs[index], fixture.practice, fixture.name, strings.ToLower(strings.ReplaceAll(fixture.name, " ", "-"))+"@example.test",
			uuid.NewString(), fixture.status, fixture.user, fixture.deleted); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO patient_registration_data (registration_id, form_version, form_data)
			VALUES ($1, 'dental-v1', jsonb_build_object('personal', jsonb_build_object('first_name', $2::text)))`, registrationIDs[index], fixture.name); err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	admin := AuthUser{ID: adminID, PracticeId: &practiceID, Role: stringPointer("admin")}
	appointments := serveDataExport(ctx, s, admin, "/export/appointments")
	if appointments.Code != http.StatusOK || !strings.Contains(appointments.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(appointments.Body.String(), "Own") || strings.Contains(appointments.Body.String(), "Deleted") || strings.Contains(appointments.Body.String(), "Other") {
		t.Fatalf("appointment export=%d headers=%v body=%s", appointments.Code, appointments.Header(), appointments.Body.String())
	}
	if !strings.Contains(appointments.Header().Get("Cache-Control"), "no-store") || !strings.Contains(appointments.Header().Get("Content-Disposition"), "appointments-") {
		t.Errorf("unsafe appointment headers: %v", appointments.Header())
	}

	registrations := serveDataExport(ctx, s, admin, "/export/registrations")
	if registrations.Code != http.StatusOK || registrations.Header().Get("Content-Type") != docxContentType {
		t.Fatalf("registration export=%d headers=%v body=%s", registrations.Code, registrations.Header(), registrations.Body.String())
	}
	registrationText, _ := readDOCXText(t, registrations.Body.Bytes())
	if !strings.Contains(registrationText, "Own Registration") || strings.Contains(registrationText, "Deleted Registration") || strings.Contains(registrationText, "Other Registration") || strings.Contains(registrationText, "Pending Registration") {
		t.Fatalf("registration tenant/status leak: %s", registrationText)
	}
	var auditCount, exportedRows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*), SUM(row_count) FROM data_export_audit_log WHERE practice_id=$1 AND exported_by_user_id=$2`, practiceID, adminID).Scan(&auditCount, &exportedRows); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 || exportedRows != 2 {
		t.Errorf("audit count=%d rows=%d", auditCount, exportedRows)
	}

	staff := AuthUser{ID: staffID, PracticeId: &practiceID, Role: stringPointer("staff")}
	staffResponse := serveDataExport(ctx, s, staff, "/export/appointments")
	if staffResponse.Code != http.StatusForbidden {
		t.Fatalf("staff export=%d %s", staffResponse.Code, staffResponse.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM data_export_audit_log WHERE exported_by_user_id=$1`, staffID).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Errorf("staff audit count=%d err=%v", auditCount, err)
	}
}

func newDataExportTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	for _, file := range []string{
		"003_appointments.sql", "007_practice_locations.sql", "008_patients.sql",
		"015_patient_registrations.sql", "016_patient_registration_data.sql",
		"020_appointments_soft_delete.sql", "023_location_address_nullable.sql",
		"027_appointment_confirmation_state.sql", "028_registration_contacts.sql",
		"031_appointment_staff_creation.sql", "034_data_export_audit.sql",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
	return pool
}

func nullableExportLocation(include bool, id uuid.UUID) *uuid.UUID {
	if include {
		return &id
	}
	return nil
}

func serveDataExport(ctx context.Context, s *Server, user AuthUser, path string) *httptest.ResponseRecorder {
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/export/appointments", setUser, requireAdmin(), s.handlerExportAppointments)
	router.GET("/export/registrations", setUser, requireAdmin(), s.handlerExportRegistrations)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	return response
}

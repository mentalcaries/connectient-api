//go:build integration

package server

import (
	"bytes"
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

func TestPatientHistoryIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyPatientHistoryTestSchema(t, ctx, pool)
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	userID := uuid.New()
	patientID, emptyPatientID, otherPatientID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'History Practice', 'Test City', 'history-practice', 'dental'),
		       ($2, 'Other History Practice', 'Test City', 'other-history-practice', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'History', 'Owner', 'history@example.test', $2, 'owner')`, userID, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO patients (id, practice_id, first_name, last_name)
		VALUES ($1, $4, 'History', 'Patient'), ($2, $4, 'Empty', 'Patient'), ($3, $5, 'Other', 'Patient')`,
		patientID, emptyPatientID, otherPatientID, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}

	newestAppointmentID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, first_name, last_name, email, mobile_phone, requested_date, requested_time,
		 appointment_type, is_scheduled, scheduled_date, scheduled_time, is_confirmed,
		 is_cancelled, practice_id, patient_id, token, created_at, deleted_at)
		VALUES
		($1, 'History', 'Patient', 'patient@example.test', '+15555550100', CURRENT_DATE, 'morning',
		 'consultation', TRUE, CURRENT_DATE + 1, '09:30', TRUE, FALSE, $4, $5, 'appointment-token-newest', NOW(), NULL),
		($2, 'History', 'Patient', 'patient@example.test', '+15555550100', CURRENT_DATE - 1, 'afternoon',
		 'checkup', FALSE, NULL, NULL, FALSE, FALSE, $4, $5, 'appointment-token-older', NOW() - INTERVAL '1 day', NULL),
		($3, 'History', 'Patient', 'patient@example.test', '+15555550100', CURRENT_DATE - 2, 'morning',
		 'deleted', FALSE, NULL, NULL, FALSE, FALSE, $4, $5, 'appointment-token-deleted', NOW() + INTERVAL '1 day', NOW())`,
		newestAppointmentID, uuid.New(), uuid.New(), practiceID, patientID); err != nil {
		t.Fatal(err)
	}
	latestRegistrationID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO patient_registrations
		(id, practice_id, patient_id, patient_name, patient_email, token, token_expires_at,
		 status, sent_by_user_id, sent_at, completed_at, created_at, deleted_at)
		VALUES
		($1, $4, $5, 'History Patient', 'patient@example.test', 'registration-token-latest', NOW() + INTERVAL '7 days',
		 'completed', $6, NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour', NOW(), NULL),
		($2, $4, $5, 'History Patient', 'patient@example.test', 'registration-token-older', NOW() + INTERVAL '7 days',
		 'pending', $6, NOW() - INTERVAL '2 days', NULL, NOW() - INTERVAL '2 days', NULL),
		($3, $4, $5, 'History Patient', 'patient@example.test', 'registration-token-deleted', NOW() + INTERVAL '7 days',
		 'pending', $6, NOW(), NULL, NOW() + INTERVAL '1 day', NOW())`,
		latestRegistrationID, uuid.New(), uuid.New(), practiceID, patientID, userID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	user := AuthUser{PracticeId: &practiceID}
	response := requestPatientHistory(ctx, s, "/patients/"+patientID.String()+"/appointments", user)
	if response.Code != http.StatusOK {
		t.Fatalf("appointment history = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("appointment cache control = %q", response.Header().Get("Cache-Control"))
	}
	body := response.Body.String()
	if !strings.Contains(body, newestAppointmentID.String()) || !strings.Contains(body, `"is_confirmed":true`) ||
		strings.Contains(body, "appointment-token") || strings.Contains(body, `"appointment_type":"deleted"`) {
		t.Errorf("unexpected appointment history: %s", body)
	}
	if strings.Index(body, `"appointment_type":"consultation"`) > strings.Index(body, `"appointment_type":"checkup"`) {
		t.Errorf("appointments not newest first: %s", body)
	}

	response = requestPatientHistory(ctx, s, "/patients/"+patientID.String()+"/registrations", user)
	if response.Code != http.StatusOK {
		t.Fatalf("registration history = %d %s", response.Code, response.Body.String())
	}
	body = response.Body.String()
	if !strings.Contains(body, latestRegistrationID.String()) || !strings.Contains(body, `"status":"completed"`) || strings.Contains(body, "registration-token") {
		t.Errorf("unexpected latest registration: %s", body)
	}
	response = requestPatientHistory(ctx, s, "/patients/"+emptyPatientID.String()+"/registrations", user)
	if response.Code != http.StatusOK || response.Body.String() != `{"data":null}` {
		t.Errorf("empty registration = %d %s", response.Code, response.Body.String())
	}
	for _, suffix := range []string{"appointments", "registrations"} {
		response = requestPatientHistory(ctx, s, "/patients/"+otherPatientID.String()+"/"+suffix, user)
		if response.Code != http.StatusNotFound || response.Body.String() != `{"error":"Patient not found"}` {
			t.Errorf("cross-tenant %s = %d %s", suffix, response.Code, response.Body.String())
		}
	}
}

func applyPatientHistoryTestSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, file := range []string{
		"003_appointments.sql", "008_patients.sql", "015_patient_registrations.sql",
		"020_appointments_soft_delete.sql", "027_appointment_confirmation_state.sql",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(data), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
}

func requestPatientHistory(ctx context.Context, s *Server, path string, user AuthUser) *httptest.ResponseRecorder {
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/patients/:id/appointments", setUser, s.handlerGetPatientAppointments)
	router.GET("/patients/:id/registrations", setUser, s.handlerGetLatestPatientRegistration)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, bytes.NewReader(nil)).WithContext(ctx)
	router.ServeHTTP(response, request)
	return response
}

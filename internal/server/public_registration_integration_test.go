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

func TestPublicRegistrationIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyRegistrationTestSchema(t, ctx, pool)
	applyPatientUniquenessOnly(t, ctx, pool)
	practiceID, suspendedID, userID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, practice_code, practice_category, logo, is_suspended)
		VALUES ($1, 'Public Registration', 'Test City', 'public-registration', 'dental', 'https://media.example/logo', FALSE),
		       ($2, 'Suspended Registration', 'Test City', 'suspended-registration', 'dental', NULL, TRUE)`, practiceID, suspendedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_settings
		(practice_id, dental_history_enabled, tmj_history_enabled, theme, theme_colors)
		VALUES ($1, TRUE, TRUE, 'custom', '{"primary":"#123456"}'::jsonb),
		       ($2, FALSE, FALSE, 'default', NULL)`, practiceID, suspendedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status, "periodEnd")
		VALUES ('pro', $1, 'active', NOW() + INTERVAL '1 day'),
		       ('pro', $2, 'active', NOW() + INTERVAL '1 day')`, practiceID.String(), suspendedID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Public', 'Owner', 'public-owner@example.test', $2, 'owner')`, userID, practiceID); err != nil {
		t.Fatal(err)
	}
	appointmentID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, requested_date, token)
		VALUES ($1, $2, 'Jane', 'Doe', 'prefill@example.test', '+15555550100', CURRENT_DATE, 'public-registration-appointment')`, appointmentID, practiceID); err != nil {
		t.Fatal(err)
	}
	insertPublicRegistration := func(practice uuid.UUID, token, status string, expires time.Time, deleted bool) uuid.UUID {
		t.Helper()
		id := uuid.New()
		var deletedAt any
		if deleted {
			deletedAt = time.Now()
		}
		if _, err := pool.Exec(ctx, `INSERT INTO patient_registrations
			(id, practice_id, appointment_id, patient_name, patient_email, token,
			 token_expires_at, status, sent_by_user_id, deleted_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, practice, appointmentID, "Jane Doe "+token[:2], token[:2]+"@example.test",
			token, expires, status, userID, deletedAt); err != nil {
			t.Fatal(err)
		}
		return id
	}
	validToken := strings.Repeat("a", 64)
	registrationID := insertPublicRegistration(practiceID, validToken, "pending", time.Now().Add(time.Hour), false)
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}

	response := requestPublicRegistration(ctx, s, http.MethodGet, validToken, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"registration_id":"`+registrationID.String()+`"`) ||
		!strings.Contains(response.Body.String(), `"theme":"custom"`) || !strings.Contains(response.Body.String(), `"email":"prefill@example.test"`) {
		t.Fatalf("bootstrap = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Errorf("cache control = %q", response.Header().Get("Cache-Control"))
	}

	response = requestPublicRegistration(ctx, s, http.MethodPost, validToken, validRegistrationSubmission)
	if response.Code != http.StatusCreated {
		t.Fatalf("submission = %d %s", response.Code, response.Body.String())
	}
	var patientID *uuid.UUID
	var status string
	if err := pool.QueryRow(ctx, `SELECT patient_id, status FROM patient_registrations WHERE id = $1`, registrationID).Scan(&patientID, &status); err != nil || patientID == nil || status != "completed" {
		t.Fatalf("registration not completed: patient=%v status=%q err=%v", patientID, status, err)
	}
	var firstName, lastName, email, phone string
	var consent bool
	if err := pool.QueryRow(ctx, `SELECT first_name, last_name, email, mobile_phone, email_consent FROM patients WHERE id = $1`, *patientID).
		Scan(&firstName, &lastName, &email, &phone, &consent); err != nil {
		t.Fatal(err)
	}
	if firstName != "Jane" || lastName != "Doe" || email != "jane@example.test" || phone != "15555550100" || consent {
		t.Errorf("unexpected patient: %q %q %q %q consent=%t", firstName, lastName, email, phone, consent)
	}
	var formCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM patient_registration_data WHERE registration_id = $1`, registrationID).Scan(&formCount); err != nil || formCount != 1 {
		t.Errorf("form data count=%d err=%v", formCount, err)
	}
	response = requestPublicRegistration(ctx, s, http.MethodPost, validToken, validRegistrationSubmission)
	if response.Code != http.StatusConflict {
		t.Errorf("repeat submission = %d %s", response.Code, response.Body.String())
	}

	expiredToken := strings.Repeat("b", 64)
	expiredID := insertPublicRegistration(practiceID, expiredToken, "pending", time.Now().Add(-time.Hour), false)
	response = requestPublicRegistration(ctx, s, http.MethodGet, expiredToken, "")
	if response.Code != http.StatusGone {
		t.Errorf("expired bootstrap = %d %s", response.Code, response.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM patient_registrations WHERE id = $1`, expiredID).Scan(&status); err != nil || status != "expired" {
		t.Errorf("expired status=%q err=%v", status, err)
	}

	deletedToken := strings.Repeat("c", 64)
	insertPublicRegistration(practiceID, deletedToken, "pending", time.Now().Add(time.Hour), true)
	response = requestPublicRegistration(ctx, s, http.MethodGet, deletedToken, "")
	if response.Code != http.StatusNotFound {
		t.Errorf("deleted token = %d, want 404", response.Code)
	}

	suspendedToken := strings.Repeat("d", 64)
	insertPublicRegistration(suspendedID, suspendedToken, "pending", time.Now().Add(time.Hour), false)
	response = requestPublicRegistration(ctx, s, http.MethodPost, suspendedToken, validRegistrationSubmission)
	if response.Code != http.StatusNotFound {
		t.Errorf("suspended submission = %d %s", response.Code, response.Body.String())
	}

	failedToken := strings.Repeat("e", 64)
	failedID := insertPublicRegistration(practiceID, failedToken, "pending", time.Now().Add(time.Hour), false)
	if _, err := pool.Exec(ctx, `ALTER TABLE patient_registration_data ADD CONSTRAINT injected_form_failure CHECK (form_version <> 'fail')`); err != nil {
		t.Fatal(err)
	}
	failingBody := strings.Replace(validRegistrationSubmission, `"medical-v1"`, `"fail"`, 1)
	response = requestPublicRegistration(ctx, s, http.MethodPost, failedToken, failingBody)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed transaction = %d %s", response.Code, response.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT status, patient_id FROM patient_registrations WHERE id = $1`, failedID).Scan(&status, &patientID); err != nil || status != "pending" || patientID != nil {
		t.Errorf("failed submission partially persisted: status=%q patient=%v err=%v", status, patientID, err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM patients WHERE email = 'jane@example.test'`).Scan(&formCount); err != nil || formCount != 1 {
		t.Errorf("failed submission created patient: count=%d err=%v", formCount, err)
	}

	concurrentToken := strings.Repeat("f", 64)
	concurrentID := insertPublicRegistration(practiceID, concurrentToken, "pending", time.Now().Add(time.Hour), false)
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			responses <- requestPublicRegistration(ctx, s, http.MethodPost, concurrentToken, validRegistrationSubmission)
		}()
	}
	statuses := []int{(<-responses).Code, (<-responses).Code}
	if !((statuses[0] == http.StatusCreated && statuses[1] == http.StatusConflict) ||
		(statuses[1] == http.StatusCreated && statuses[0] == http.StatusConflict)) {
		t.Errorf("concurrent statuses = %v, want 201 and 409", statuses)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM patient_registration_data WHERE registration_id = $1`, concurrentID).Scan(&formCount); err != nil || formCount != 1 {
		t.Errorf("concurrent form count=%d err=%v", formCount, err)
	}
}

func applyPatientUniquenessOnly(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", "026_patient_contact_uniqueness.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}
}

func requestPublicRegistration(ctx context.Context, s *Server, method, token, body string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/registrations/form/:token", s.handlerGetPublicRegistrationForm)
	router.POST("/registrations/form/:token", s.handlerSubmitPublicRegistrationForm)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/registrations/form/"+token, bytes.NewBufferString(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

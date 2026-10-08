//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
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

type fakeRegistrationNotifier struct{ emailCalls, whatsappCalls int }

func (f *fakeRegistrationNotifier) SendRegistrationEmail(context.Context, RegistrationNotification) (RegistrationDeliveryResult, error) {
	f.emailCalls++
	return RegistrationDeliverySent, nil
}

func (f *fakeRegistrationNotifier) SendRegistrationWhatsApp(context.Context, RegistrationNotification) (RegistrationDeliveryResult, error) {
	f.whatsappCalls++
	return RegistrationDeliverySent, nil
}

func TestRegistrationsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyRegistrationTestSchema(t, ctx, pool)
	practiceID, otherPracticeID, userID := uuid.New(), uuid.New(), uuid.New()
	owner := "owner"
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Registration Practice', 'Test City', 'registration-practice', 'dental'),
		       ($2, 'Other Practice', 'Test City', 'other-registration-practice', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Registration', 'Owner', 'owner@example.test', $2, 'owner')`, userID, practiceID); err != nil {
		t.Fatal(err)
	}
	appointmentID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, requested_date, token)
		VALUES ($1, $2, 'Jane', 'Doe', 'jane@example.test', '+15555550100', CURRENT_DATE, 'appointment-registration-token')`, appointmentID, practiceID); err != nil {
		t.Fatal(err)
	}
	notifier := &fakeRegistrationNotifier{}
	s := &Server{
		db: registrationTestDB{pool}, DBQuery: db.New(pool), registrationNotify: notifier,
		patientBaseURL: "https://patient.example.test",
	}
	user := AuthUser{ID: userID, PracticeId: &practiceID, Role: &owner}

	response := requestRegistration(ctx, s, http.MethodPost, "/registrations", user, map[string]any{
		"patient_name": "Jane Doe", "appointment_id": appointmentID,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create registration = %d %s", response.Code, response.Body.String())
	}
	var created struct {
		ID               uuid.UUID `json:"id"`
		Token            string    `json:"token"`
		Link             string    `json:"link"`
		NotificationSent *bool     `json:"notification_sent"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Token) != 64 || !strings.HasSuffix(created.Link, created.Token) || created.NotificationSent != nil {
		t.Errorf("unexpected create response: %+v", created)
	}
	var email, phone *string
	if err := pool.QueryRow(ctx, `SELECT patient_email, patient_phone FROM patient_registrations WHERE id = $1`, created.ID).Scan(&email, &phone); err != nil {
		t.Fatal(err)
	}
	if email == nil || *email != "jane@example.test" || phone == nil || *phone != "+15555550100" {
		t.Errorf("appointment contacts not used: email=%v phone=%v", email, phone)
	}

	response = requestRegistration(ctx, s, http.MethodPost, "/registrations", user, map[string]any{
		"patient_name": "Jane Doe", "patient_email": "jane@example.test",
	})
	if response.Code != http.StatusConflict {
		t.Errorf("duplicate registration = %d %s", response.Code, response.Body.String())
	}

	response = requestRegistration(ctx, s, http.MethodPost, "/registrations", user, map[string]any{
		"patient_name": "Email Patient", "patient_email": "email@example.test", "send_email": true,
	})
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"notification_sent":true`) || notifier.emailCalls != 1 {
		t.Fatalf("email create = %d %s calls=%d", response.Code, response.Body.String(), notifier.emailCalls)
	}
	var sentAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT sent_at FROM patient_registrations WHERE patient_name = 'Email Patient'`).Scan(&sentAt); err != nil || sentAt == nil {
		t.Errorf("sent_at not tracked: %v %v", sentAt, err)
	}

	response = requestRegistration(ctx, s, http.MethodGet, "/registrations?search=Jane", user, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"patient_name":"Jane Doe"`) || strings.Contains(response.Body.String(), `"PatientName"`) {
		t.Fatalf("registration list = %d %s", response.Code, response.Body.String())
	}

	if _, err := pool.Exec(ctx, `UPDATE patient_registrations SET status = 'completed', completed_at = NOW() WHERE id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO patient_registration_data (registration_id, form_version, form_data)
		VALUES ($1, '1', '{"personal":{"first_name":"Jane"}}'::jsonb)`, created.ID); err != nil {
		t.Fatal(err)
	}
	response = requestRegistration(ctx, s, http.MethodGet, "/registrations/"+created.ID.String(), user, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"form_version":"1"`) || !strings.Contains(response.Body.String(), `"first_name":"Jane"`) {
		t.Fatalf("registration detail = %d %s", response.Code, response.Body.String())
	}
	response = requestRegistration(ctx, s, http.MethodDelete, "/registrations/"+created.ID.String(), user, nil)
	if response.Code != http.StatusBadRequest {
		t.Errorf("completed delete = %d, want 400", response.Code)
	}

	expiredID, oldToken := uuid.New(), strings.Repeat("a", 64)
	if _, err := pool.Exec(ctx, `INSERT INTO patient_registrations
		(id, practice_id, patient_name, patient_email, token, token_expires_at, status, sent_by_user_id)
		VALUES ($1, $2, 'Expired Patient', 'expired@example.test', $3, NOW() - INTERVAL '1 day', 'expired', $4)`,
		expiredID, practiceID, oldToken, userID); err != nil {
		t.Fatal(err)
	}
	response = requestRegistration(ctx, s, http.MethodGet, "/registrations/"+expiredID.String()+"/link", user, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"was_regenerated":true`) || strings.Contains(response.Body.String(), oldToken) {
		t.Fatalf("expired link = %d %s", response.Code, response.Body.String())
	}
	response = requestRegistration(ctx, s, http.MethodDelete, "/registrations/"+expiredID.String(), user, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("pending delete = %d %s", response.Code, response.Body.String())
	}
	response = requestRegistration(ctx, s, http.MethodPost, "/registrations", user, map[string]any{
		"patient_name": "Expired Patient", "patient_email": "expired@example.test",
	})
	if response.Code != http.StatusConflict {
		t.Errorf("deleted duplicate = %d %s", response.Code, response.Body.String())
	}

	otherID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO patient_registrations
		(id, practice_id, patient_name, patient_email, token, token_expires_at, status, sent_by_user_id)
		VALUES ($1, $2, 'Other Patient', 'other@example.test', $3, NOW() + INTERVAL '7 days', 'pending', $4)`,
		otherID, otherPracticeID, strings.Repeat("b", 64), userID); err != nil {
		t.Fatal(err)
	}
	response = requestRegistration(ctx, s, http.MethodGet, "/registrations/"+otherID.String(), user, nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant detail = %d, want 404", response.Code)
	}
}

func applyRegistrationTestSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, file := range []string{
		"003_appointments.sql", "008_patients.sql", "015_patient_registrations.sql",
		"016_patient_registration_data.sql", "020_appointments_soft_delete.sql",
		"027_appointment_confirmation_state.sql", "028_registration_contacts.sql",
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

func requestRegistration(ctx context.Context, s *Server, method, path string, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/registrations", setUser, s.handlerListRegistrations)
	router.POST("/registrations", setUser, s.handlerCreateRegistration)
	router.GET("/registrations/:id", setUser, s.handlerGetRegistration)
	router.DELETE("/registrations/:id", setUser, requireAdmin(), s.handlerDeleteRegistration)
	router.GET("/registrations/:id/link", setUser, s.handlerGetRegistrationLink)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

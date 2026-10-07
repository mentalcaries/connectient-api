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
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type fakePublicAppointmentRequestNotifier struct {
	mu           sync.Mutex
	calls        int
	notification PublicAppointmentRequestNotification
}

func (f *fakePublicAppointmentRequestNotifier) NotifyStaffAppointmentRequest(_ context.Context, notification PublicAppointmentRequestNotification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.notification = notification
	return nil
}

func TestPublicAppointmentRequestIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newPublicAppointmentRequestTestPool(t, ctx)
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	providerID, otherProviderID, locationID := uuid.New(), uuid.New(), uuid.New()
	practiceEmail := "staff@example.test"
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, email, practice_code, practice_category, is_active)
		VALUES ($1, 'Booking Practice', 'Test City', $3, 'booking-practice', 'dental', TRUE),
		       ($2, 'Other Practice', 'Test City', 'other@example.test', 'other-booking', 'dental', TRUE)`, practiceID, otherPracticeID, practiceEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_settings
		(practice_id, multiple_locations_enabled, available_weekdays)
		VALUES ($1, TRUE, ARRAY[0,1,2,3,4,5,6]), ($2, FALSE, ARRAY[0,1,2,3,4,5,6])`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status, "trialStart", "trialEnd")
		VALUES ('pro', $1, 'trialing', NOW(), NOW() + INTERVAL '30 days'),
		       ('pro', $2, 'trialing', NOW(), NOW() + INTERVAL '30 days')`, practiceID.String(), otherPracticeID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO procedure_types (practice_id, name, value, is_active)
		VALUES ($1, 'Consultation', 'consultation', TRUE), ($2, 'Other', 'other', TRUE)`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO provider (id, first_name, last_name, specialty)
		VALUES ($1, 'Primary', 'Provider', 'General'), ($2, 'Other', 'Provider', 'General')`, providerID, otherProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_provider (practice_id, provider_id, is_main)
		VALUES ($1, $2, TRUE), ($3, $4, TRUE)`, practiceID, providerID, otherPracticeID, otherProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_locations
		(id, practice_id, name, is_active, available_weekdays) VALUES ($1, $2, 'Main', TRUE, ARRAY[0,1,2,3,4,5,6])`, locationID, practiceID); err != nil {
		t.Fatal(err)
	}

	notifier := &fakePublicAppointmentRequestNotifier{}
	events := &fakeAppointmentServices{}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool), publicBookingNotify: notifier, appointmentEvents: events}
	body := publicAppointmentFixture(providerID, locationID)

	response := requestPublicAppointment(t, ctx, s, "booking-practice", "request-0001", body)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	firstBody := response.Body.String()
	var appointmentID, patientID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id, patient_id FROM appointments WHERE practice_id = $1`, practiceID).Scan(&appointmentID, &patientID); err != nil {
		t.Fatal(err)
	}
	if notifier.calls != 1 || notifier.notification.PracticeName != "Booking Practice" ||
		notifier.notification.PracticeEmail == nil || *notifier.notification.PracticeEmail != practiceEmail || events.broadcasts != 1 {
		t.Errorf("unexpected post-commit effects: notifier=%+v events=%+v", notifier, events)
	}

	response = requestPublicAppointment(t, ctx, s, "booking-practice", "request-0001", body)
	if response.Code != http.StatusCreated || response.Header().Get("Idempotent-Replayed") != "true" || response.Body.String() != firstBody {
		t.Fatalf("replay = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if notifier.calls != 1 || events.broadcasts != 1 {
		t.Errorf("replay repeated side effects: notifier=%d broadcasts=%d", notifier.calls, events.broadcasts)
	}
	body["description"] = "Changed"
	response = requestPublicAppointment(t, ctx, s, "booking-practice", "request-0001", body)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "different request") {
		t.Fatalf("changed replay = %d %s", response.Code, response.Body.String())
	}

	concurrentBody := publicAppointmentFixture(providerID, locationID)
	concurrentBody["email"] = "concurrent@example.test"
	concurrentBody["mobile_phone"] = "+1 (555) 123-4568"
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			<-start
			responses <- requestPublicAppointment(t, ctx, s, "booking-practice", "request-concurrent", concurrentBody)
		}()
	}
	close(start)
	replayed := 0
	for range 2 {
		concurrentResponse := <-responses
		if concurrentResponse.Code != http.StatusCreated {
			t.Errorf("concurrent response = %d %s", concurrentResponse.Code, concurrentResponse.Body.String())
		}
		if concurrentResponse.Header().Get("Idempotent-Replayed") == "true" {
			replayed++
		}
	}
	if replayed != 1 || notifier.calls != 2 || events.broadcasts != 2 {
		t.Errorf("concurrent replay=%d notifier=%d broadcasts=%d", replayed, notifier.calls, events.broadcasts)
	}
	var concurrentCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM appointments WHERE practice_id = $1 AND email = 'concurrent@example.test'`, practiceID).Scan(&concurrentCount); err != nil || concurrentCount != 1 {
		t.Errorf("concurrent appointment count=%d err=%v", concurrentCount, err)
	}

	// Shared guardian contacts with a different child name create an unlinked request,
	// never merge it into the first child's patient record.
	body = publicAppointmentFixture(providerID, locationID)
	body["first_name"], body["last_name"] = "Other", "Child"
	response = requestPublicAppointment(t, ctx, s, "booking-practice", "request-0002", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("shared-contact child = %d %s", response.Code, response.Body.String())
	}
	var siblingPatientID *uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT patient_id FROM appointments WHERE practice_id = $1 AND first_name = 'Other'`, practiceID).Scan(&siblingPatientID); err != nil {
		t.Fatal(err)
	}
	if siblingPatientID != nil {
		t.Errorf("different child was linked to patient %s", *siblingPatientID)
	}
	var guardianPatientCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM patients WHERE practice_id = $1 AND email = 'guardian@example.test'`, practiceID).Scan(&guardianPatientCount); err != nil || guardianPatientCount != 1 {
		t.Errorf("guardian patient count=%d err=%v", guardianPatientCount, err)
	}

	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"cross-practice provider", func(body map[string]any) { body["preferred_provider_id"] = otherProviderID.String() }},
		{"missing location", func(body map[string]any) { body["location_id"] = nil }},
		{"unknown procedure", func(body map[string]any) { body["appointment_type"] = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := publicAppointmentFixture(providerID, locationID)
			tc.mutate(invalid)
			response := requestPublicAppointment(t, ctx, s, "booking-practice", "invalid-"+strings.ReplaceAll(tc.name, " ", "-"), invalid)
			if response.Code != http.StatusBadRequest {
				t.Errorf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
	_ = appointmentID
	_ = patientID
}

func newPublicAppointmentRequestTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	for _, file := range []string{
		"003_appointments.sql", "006_provider.sql", "007_practice_locations.sql", "008_patients.sql",
		"009_practice_provider.sql", "019_appointments_fkeys.sql", "020_appointments_soft_delete.sql",
		"022_available_weekdays.sql", "023_location_address_nullable.sql", "025_provider_ownership.sql",
		"026_patient_contact_uniqueness.sql", "027_appointment_confirmation_state.sql",
		"031_appointment_staff_creation.sql", "032_public_appointment_idempotency.sql",
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

func publicAppointmentFixture(providerID, locationID uuid.UUID) map[string]any {
	return map[string]any{
		"first_name": "Jane", "last_name": "Child", "email": "guardian@example.test",
		"mobile_phone": "+1 (555) 123-4567", "requested_date": "2099-10-08",
		"requested_time": "flexible", "appointment_type": "consultation",
		"description": "Checkup", "is_emergency": false,
		"preferred_provider_id": providerID.String(), "location_id": locationID.String(),
	}
}

func requestPublicAppointment(t *testing.T, ctx context.Context, s *Server, code, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/public/practices/:code/appointment-requests", s.handlerCreatePublicAppointmentRequest)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/public/practices/"+code+"/appointment-requests", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	router.ServeHTTP(response, request)
	return response
}

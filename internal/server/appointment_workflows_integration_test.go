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

type fakeAppointmentServices struct {
	mu              sync.Mutex
	broadcasts      int
	calendarCreates int
	calendarUpdates int
	emails          int
	whatsApps       int
}

func (f *fakeAppointmentServices) BroadcastAppointmentChange(context.Context, uuid.UUID, string, uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts++
	return nil
}
func (f *fakeAppointmentServices) SyncAppointmentCreated(context.Context, AppointmentEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calendarCreates++
	return nil
}
func (f *fakeAppointmentServices) SyncAppointmentUpdated(context.Context, AppointmentEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calendarUpdates++
	return nil
}
func (f *fakeAppointmentServices) SyncAppointmentCancelled(context.Context, uuid.UUID, uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calendarUpdates++
	return nil
}
func (f *fakeAppointmentServices) SendAppointmentEmail(context.Context, AppointmentNotification) (RegistrationDeliveryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emails++
	return RegistrationDeliverySent, nil
}
func (f *fakeAppointmentServices) SendAppointmentWhatsApp(context.Context, AppointmentNotification) (RegistrationDeliveryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.whatsApps++
	return RegistrationDeliverySimulated, nil
}

func TestAppointmentWorkflowsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newAppointmentWorkflowTestPool(t, ctx)
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	userID, providerID, otherProviderID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Schedule Practice', 'Test City', 'schedule-practice', 'dental'),
		       ($2, 'Other Practice', 'Test City', 'other-schedule', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Staff', 'User', 'staff@example.test', $2, 'staff')`, userID, practiceID); err != nil {
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
	if _, err := pool.Exec(ctx, `INSERT INTO practice_settings (practice_id, available_weekdays, multiple_locations_enabled)
		VALUES ($1, ARRAY[0,1,2,3,4,5,6], FALSE)`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status, "trialStart", "trialEnd")
		VALUES ('pro', $1, 'trialing', NOW(), NOW() + INTERVAL '30 days')`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	conflictID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, appointment_type,
		 provider_id, duration_minutes, scheduled_date, scheduled_time, is_scheduled, is_confirmed)
		VALUES ($1, $2, 'Busy', 'Patient', 'busy@example.test', '15550000001', 'Exam',
		 $3, 30, '2026-10-08', '08:00', TRUE, FALSE)`, conflictID, practiceID, providerID); err != nil {
		t.Fatal(err)
	}

	services := &fakeAppointmentServices{}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool), appointmentEvents: services, appointmentNotify: services}
	role := "staff"
	user := AuthUser{ID: userID, PracticeId: &practiceID, Role: &role}

	response := requestAppointmentWorkflow(ctx, s, user, http.MethodGet,
		"/appointments/availability?start=2026-10-08&days=1&providerId="+providerID.String()+"&durationMinutes=30", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"conflict"`) || !strings.Contains(response.Body.String(), conflictID.String()) {
		t.Fatalf("availability = %d %s", response.Code, response.Body.String())
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodGet,
		"/appointments/availability?start=2026-10-08&days=1&providerId="+otherProviderID.String()+"&durationMinutes=30", nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-practice provider = %d", response.Code)
	}

	createBody := map[string]any{
		"patient":         map[string]any{"kind": "new", "firstName": "Jane", "lastName": "Doe", "email": "jane@example.test", "mobilePhone": "+1 (555) 000-0002"},
		"appointmentType": "Consultation", "scheduledDate": "2026-10-08", "scheduledTime": "08:00",
		"durationMinutes": 30, "providerId": providerID.String(), "isConfirmed": true,
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments", createBody)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), conflictID.String()) {
		t.Fatalf("create conflict = %d %s", response.Code, response.Body.String())
	}
	var patientCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM patients WHERE practice_id = $1`, practiceID).Scan(&patientCount); err != nil || patientCount != 0 {
		t.Errorf("conflict mutated patients: count=%d err=%v", patientCount, err)
	}
	createBody["acknowledgedConflictIds"] = []string{conflictID.String()}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments", createBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"scheduled":true`) || !strings.Contains(response.Body.String(), `"email":"sent"`) || !strings.Contains(response.Body.String(), `"whatsapp":"simulated"`) {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	var createdAppointmentID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM appointments WHERE patient_id IS NOT NULL AND first_name = 'Jane'`).Scan(&createdAppointmentID); err != nil {
		t.Fatal(err)
	}
	createBody["scheduledTime"] = "10:00"
	createBody["acknowledgedConflictIds"] = []string{}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments", createBody)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"patient_exists"`) {
		t.Fatalf("duplicate patient = %d %s", response.Code, response.Body.String())
	}

	requestID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments (id, practice_id, first_name, last_name, email, mobile_phone)
		VALUES ($1, $2, 'Request', 'Patient', 'request@example.test', '15550000003')`, requestID, practiceID); err != nil {
		t.Fatal(err)
	}
	scheduleBody := map[string]any{"scheduledDate": "2026-10-08", "scheduledTime": "08:15", "durationMinutes": 15, "providerId": providerID.String()}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/schedule", scheduleBody)
	if response.Code != http.StatusConflict {
		t.Fatalf("schedule conflict = %d %s", response.Code, response.Body.String())
	}
	scheduleBody["acknowledgedConflictIds"] = []string{conflictID.String(), createdAppointmentID.String()}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/schedule", scheduleBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"scheduled":true`) {
		t.Fatalf("schedule = %d %s", response.Code, response.Body.String())
	}

	confirmBody := map[string]any{"sendEmail": true, "notifyEmail": "confirmed@example.test", "sendWhatsApp": true, "notifyPhone": "+15550000004"}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/confirm", confirmBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"is_confirmed":true`) {
		t.Fatalf("confirm = %d %s", response.Code, response.Body.String())
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/confirm", confirmBody)
	if response.Code != http.StatusConflict {
		t.Errorf("second confirm = %d %s", response.Code, response.Body.String())
	}
	if services.calendarCreates != 2 || services.calendarUpdates != 0 || services.broadcasts != 3 || services.emails != 2 || services.whatsApps != 2 {
		t.Errorf("unexpected side effects: %+v", services)
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/cancel", map[string]any{})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"is_cancelled":true`) {
		t.Fatalf("cancel = %d %s", response.Code, response.Body.String())
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/cancel", map[string]any{})
	if response.Code != http.StatusOK {
		t.Fatalf("idempotent cancel = %d %s", response.Code, response.Body.String())
	}
	if services.calendarUpdates != 1 || services.broadcasts != 4 || services.emails != 2 || services.whatsApps != 2 {
		t.Errorf("cancellation side effects repeated or notified patient: %+v", services)
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+requestID.String()+"/schedule", scheduleBody)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Cancelled appointments cannot be scheduled") {
		t.Fatalf("cancelled reschedule = %d %s", response.Code, response.Body.String())
	}
	otherUser := user
	otherUser.PracticeId = &otherPracticeID
	response = requestAppointmentWorkflow(ctx, s, otherUser, http.MethodPost, "/appointments/"+requestID.String()+"/cancel", map[string]any{})
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-practice cancel = %d %s", response.Code, response.Body.String())
	}
	concurrentCancelID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, is_scheduled, scheduled_date, scheduled_time)
		VALUES ($1, $2, 'Concurrent', 'Cancel', 'cancel@example.test', '15550000005', TRUE, '2026-10-09', '09:00')`, concurrentCancelID, practiceID); err != nil {
		t.Fatal(err)
	}
	startCancel := make(chan struct{})
	cancelResponses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() {
			<-startCancel
			cancelResponses <- requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+concurrentCancelID.String()+"/cancel", map[string]any{})
		}()
	}
	close(startCancel)
	for range 2 {
		if concurrentResponse := <-cancelResponses; concurrentResponse.Code != http.StatusOK {
			t.Errorf("concurrent cancel = %d %s", concurrentResponse.Code, concurrentResponse.Body.String())
		}
	}
	if services.calendarUpdates != 2 || services.broadcasts != 5 {
		t.Errorf("concurrent cancellation repeated side effects: %+v", services)
	}
	deletedID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, deleted_at)
		VALUES ($1, $2, 'Deleted', 'Request', 'deleted@example.test', '15550000006', NOW())`, deletedID, practiceID); err != nil {
		t.Fatal(err)
	}
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments/"+deletedID.String()+"/cancel", map[string]any{})
	if response.Code != http.StatusNotFound {
		t.Errorf("soft-deleted cancel = %d %s", response.Code, response.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE subscription SET status = 'expired', "trialEnd" = NOW() - INTERVAL '31 days' WHERE "referenceId" = $1`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	createBody["scheduledTime"] = "11:00"
	response = requestAppointmentWorkflow(ctx, s, user, http.MethodPost, "/appointments", createBody)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "active subscription") {
		t.Errorf("inactive subscription = %d %s", response.Code, response.Body.String())
	}
}

func newAppointmentWorkflowTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	for _, file := range []string{
		"003_appointments.sql", "006_provider.sql", "007_practice_locations.sql", "008_patients.sql",
		"009_practice_provider.sql", "019_appointments_fkeys.sql", "020_appointments_soft_delete.sql",
		"022_available_weekdays.sql", "023_location_address_nullable.sql", "025_provider_ownership.sql",
		"026_patient_contact_uniqueness.sql", "027_appointment_confirmation_state.sql", "031_appointment_staff_creation.sql",
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

func requestAppointmentWorkflow(ctx context.Context, s *Server, user AuthUser, method, path string, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/appointments/availability", setUser, s.handlerGetAppointmentAvailability)
	router.POST("/appointments", setUser, s.handlerCreateStaffAppointment)
	router.POST("/appointments/:id/schedule", setUser, s.handlerScheduleAppointment)
	router.POST("/appointments/:id/confirm", setUser, s.handlerConfirmAppointment)
	router.POST("/appointments/:id/cancel", setUser, s.handlerCancelAppointment)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

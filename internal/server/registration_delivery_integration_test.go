//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type scriptedRegistrationNotifier struct {
	emailResult    RegistrationDeliveryResult
	whatsappResult RegistrationDeliveryResult
	emailCalls     int
	whatsappCalls  int
}

func (f *scriptedRegistrationNotifier) SendRegistrationEmail(context.Context, RegistrationNotification) (RegistrationDeliveryResult, error) {
	f.emailCalls++
	return f.emailResult, nil
}

func (f *scriptedRegistrationNotifier) SendRegistrationWhatsApp(context.Context, RegistrationNotification) (RegistrationDeliveryResult, error) {
	f.whatsappCalls++
	return f.whatsappResult, nil
}

func TestRegistrationDeliveryIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyRegistrationTestSchema(t, ctx, pool)
	practiceID, otherPracticeID, userID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Delivery Practice', 'Test City', 'delivery-practice', 'dental'),
		       ($2, 'Other Delivery', 'Test City', 'other-delivery', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Delivery', 'Owner', 'delivery@example.test', $2, 'owner')`, userID, practiceID); err != nil {
		t.Fatal(err)
	}
	appointmentID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, requested_date, token)
		VALUES ($1, $2, 'Phone', 'Fallback', 'fallback@example.test', '+15555550199', CURRENT_DATE, 'delivery-appointment-token')`, appointmentID, practiceID); err != nil {
		t.Fatal(err)
	}
	insertRegistration := func(name, email string, phone *string, expires time.Time, status string) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO patient_registrations
			(id, practice_id, appointment_id, patient_name, patient_email, patient_phone,
			 token, token_expires_at, status, sent_by_user_id, sent_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW() - INTERVAL '1 hour')`,
			id, practiceID, appointmentID, name, email, phone,
			strings.ReplaceAll(uuid.NewString(), "-", "")+strings.ReplaceAll(uuid.NewString(), "-", ""),
			expires, status, userID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	notifier := &scriptedRegistrationNotifier{
		emailResult: RegistrationDeliverySent, whatsappResult: RegistrationDeliverySimulated,
	}
	s := &Server{
		db: registrationTestDB{pool}, DBQuery: db.New(pool), registrationNotify: notifier,
		patientBaseURL: "https://patient.example.test",
	}
	role := "owner"
	user := AuthUser{ID: userID, PracticeId: &practiceID, Role: &role}

	emailID := insertRegistration("Email Success", "old-email@example.test", nil, time.Now().Add(time.Hour), "pending")
	response := requestRegistrationDelivery(ctx, s, "/registrations/"+emailID.String()+"/send-email", user, map[string]any{"email": "new-email@example.test"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"email":"new-email@example.test"`) {
		t.Fatalf("send email = %d %s", response.Code, response.Body.String())
	}
	var savedEmail *string
	var sentAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT patient_email, sent_at FROM patient_registrations WHERE id = $1`, emailID).Scan(&savedEmail, &sentAt); err != nil || savedEmail == nil || *savedEmail != "new-email@example.test" || sentAt == nil {
		t.Errorf("email delivery not persisted: email=%v sent=%v err=%v", savedEmail, sentAt, err)
	}

	notifier.emailResult = RegistrationDeliveryFailed
	failedID := insertRegistration("Email Failure", "previous@example.test", nil, time.Now().Add(time.Hour), "pending")
	response = requestRegistrationDelivery(ctx, s, "/registrations/"+failedID.String()+"/send-email", user, map[string]any{"email": "failed@example.test"})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed email = %d %s", response.Code, response.Body.String())
	}
	if err := pool.QueryRow(ctx, `SELECT patient_email FROM patient_registrations WHERE id = $1`, failedID).Scan(&savedEmail); err != nil || savedEmail == nil || *savedEmail != "previous@example.test" {
		t.Errorf("email was not restored: %v err=%v", savedEmail, err)
	}

	phoneID := insertRegistration("Phone Success", "phone@example.test", nil, time.Now().Add(time.Hour), "pending")
	response = requestRegistrationDelivery(ctx, s, "/registrations/"+phoneID.String()+"/send-whatsapp", user, nil)
	if response.Code != http.StatusOK || notifier.whatsappCalls != 1 {
		t.Fatalf("send WhatsApp = %d %s calls=%d", response.Code, response.Body.String(), notifier.whatsappCalls)
	}
	var savedPhone *string
	if err := pool.QueryRow(ctx, `SELECT patient_phone FROM patient_registrations WHERE id = $1`, phoneID).Scan(&savedPhone); err != nil || savedPhone == nil || *savedPhone != "+15555550199" {
		t.Errorf("appointment phone fallback not saved: %v err=%v", savedPhone, err)
	}

	expiredID := insertRegistration("Expired Link", "expired-link@example.test", nil, time.Now().Add(-time.Hour), "expired")
	callsBefore := notifier.emailCalls
	response = requestRegistrationDelivery(ctx, s, "/registrations/"+expiredID.String()+"/send-email", user, nil)
	if response.Code != http.StatusGone || notifier.emailCalls != callsBefore {
		t.Errorf("expired send = %d %s calls=%d", response.Code, response.Body.String(), notifier.emailCalls)
	}

	resendID := insertRegistration("Resend Patient", "resend@example.test", nil, time.Now().Add(-time.Hour), "expired")
	var previousToken, previousStatus string
	var previousExpiry time.Time
	if err := pool.QueryRow(ctx, `SELECT token, token_expires_at, status FROM patient_registrations WHERE id = $1`, resendID).Scan(&previousToken, &previousExpiry, &previousStatus); err != nil {
		t.Fatal(err)
	}
	notifier.emailResult = RegistrationDeliveryFailed
	response = requestRegistrationDelivery(ctx, s, "/registrations/"+resendID.String()+"/resend", user, nil)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed resend = %d %s", response.Code, response.Body.String())
	}
	var token, status string
	var expiry time.Time
	if err := pool.QueryRow(ctx, `SELECT token, token_expires_at, status FROM patient_registrations WHERE id = $1`, resendID).Scan(&token, &expiry, &status); err != nil || token != previousToken || status != previousStatus || !expiry.Equal(previousExpiry) {
		t.Errorf("resend state not restored: token=%q status=%q expiry=%v err=%v", token, status, expiry, err)
	}
	notifier.emailResult = RegistrationDeliverySent
	response = requestRegistrationDelivery(ctx, s, "/registrations/"+resendID.String()+"/resend", user, nil)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), previousToken) || !strings.Contains(response.Body.String(), `"status":"pending"`) {
		t.Fatalf("successful resend = %d %s", response.Code, response.Body.String())
	}
}

func requestRegistrationDelivery(ctx context.Context, s *Server, path string, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.POST("/registrations/:id/send-email", setUser, s.handlerSendRegistrationEmail)
	router.POST("/registrations/:id/send-whatsapp", setUser, s.handlerSendRegistrationWhatsApp)
	router.POST("/registrations/:id/resend", setUser, s.handlerResendRegistration)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

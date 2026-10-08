//go:build integration

package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestPublicAppointmentNotificationProviderIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", "013_notification_log.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}
	practiceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, email, practice_code, practice_category)
		VALUES ($1, 'Notify Practice', 'Test City', 'staff@example.test', 'notify-practice', 'dental')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users
		(id, first_name, last_name, email, mobile_phone, practice_id, role, whatsapp_notifications_enabled, is_active)
		VALUES ($1, 'Eligible', 'Staff', 'eligible@example.test', '+15550000001', $4, 'staff', TRUE, TRUE),
		       ($2, 'Opted', 'Out', 'optout@example.test', '+15550000002', $4, 'staff', FALSE, TRUE),
		       ($3, 'Deleted', 'Staff', 'deleted@example.test', '+15550000003', $4, 'staff', TRUE, TRUE)`, uuid.New(), uuid.New(), uuid.New(), practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE email = 'deleted@example.test'`); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	requests := 0
	provider := testOutboundProvider(map[string]string{
		"RESEND_API_KEY": "resend-secret", "TWILIO_ACCOUNT_SID": "AC123",
		"TWILIO_AUTH_TOKEN": "twilio-secret", "TWILIO_WHATSAPP_NUMBER": "+15559999999",
		"TWILIO_WHATSAPP_APPT_TEMPLATE": "HX123",
	}, func(*http.Request) (*http.Response, error) {
		mu.Lock()
		requests++
		mu.Unlock()
		return testHTTPResponse(http.StatusCreated), nil
	})
	provider.queries = db.New(pool)
	practiceEmail := "staff@example.test"
	if err := provider.NotifyStaffAppointmentRequest(ctx, PublicAppointmentRequestNotification{
		AppointmentID: uuid.New(), PracticeID: practiceID, PracticeName: "Notify Practice", PracticeEmail: &practiceEmail,
	}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Errorf("provider requests=%d, want email plus one eligible WhatsApp", requests)
	}
	var logs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM notification_log WHERE practice_id = $1`, practiceID).Scan(&logs); err != nil || logs != 2 {
		t.Errorf("notification logs=%d err=%v", logs, err)
	}
}

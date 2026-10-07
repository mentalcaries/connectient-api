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
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestAppointmentPatchIntegration(t *testing.T) {
	config := registrationTestConfig(t)
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"valid update", http.StatusOK},
		{"invalid ID", http.StatusBadRequest},
		{"invalid JSON", http.StatusBadRequest},
		{"missing appointment", http.StatusNotFound},
		{"other practice", http.StatusNotFound},
		{"deleted appointment", http.StatusNotFound},
		{"missing practice", http.StatusForbidden},
		{"database error", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			t.Cleanup(cancel)
			pool := newRegistrationTestPool(t, ctx, config)
			for _, file := range []string{"003_appointments.sql", "020_appointments_soft_delete.sql", "027_appointment_confirmation_state.sql"} {
				data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
					t.Fatal(err)
				}
			}
			practiceID, otherPracticeID, appointmentID := uuid.New(), uuid.New(), uuid.New()
			for _, id := range []uuid.UUID{practiceID, otherPracticeID} {
				if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
					VALUES ($1, 'Fixture', 'Test City', $2, 'dental')`, id, id.String()); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pool.Exec(ctx, `INSERT INTO appointments
				(id, practice_id, first_name, last_name, email, mobile_phone, requested_date, token)
				VALUES ($1, $2, 'Test', 'Patient', 'fixture@example.test', '+15555550100', '2026-10-01', $3)`,
				appointmentID, practiceID, uuid.NewString()); err != nil {
				t.Fatal(err)
			}

			user := AuthUser{PracticeId: &practiceID}
			path := "/appointments/" + appointmentID.String()
			// Body identity/practice and scheduling fields must never override authenticated/path scope.
			body := `{"email":" updated@example.test ","mobile_phone":" +15555550199 ","is_scheduled":true,"id":"` + uuid.NewString() + `","practice_id":"` + practiceID.String() + `"}`
			switch tc.name {
			case "invalid ID":
				path = "/appointments/not-a-uuid"
			case "invalid JSON":
				body = "{"
			case "missing appointment":
				path = "/appointments/" + uuid.NewString()
			case "other practice":
				user.PracticeId = &otherPracticeID
			case "deleted appointment":
				if _, err := pool.Exec(ctx, `UPDATE appointments SET deleted_at = NOW() WHERE id = $1`, appointmentID); err != nil {
					t.Fatal(err)
				}
			case "missing practice":
				user.PracticeId = nil
			case "database error":
				if _, err := pool.Exec(ctx, `ALTER TABLE appointments ADD CONSTRAINT injected_failure CHECK (email <> 'updated@example.test')`); err != nil {
					t.Fatal(err)
				}
			}
			var before string
			if err := pool.QueryRow(ctx, `SELECT row_to_json(a)::text FROM appointments a WHERE id = $1`, appointmentID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			s := &Server{DBQuery: db.New(pool)}
			router := gin.New()
			router.PATCH("/appointments/:id", func(c *gin.Context) {
				c.Set("user", user)
				s.handlerAppointmentsUpdate(c)
			})
			request := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(body)).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
			if tc.status == http.StatusOK {
				if response.Body.String() != `{"success":true}` {
					t.Fatalf("unexpected response: %s", response.Body.String())
				}
				var email, phone string
				var scheduled bool
				if err := pool.QueryRow(ctx, `SELECT email, mobile_phone, is_scheduled FROM appointments WHERE id = $1`, appointmentID).Scan(&email, &phone, &scheduled); err != nil {
					t.Fatal(err)
				}
				if email != "updated@example.test" || phone != "+15555550199" || scheduled {
					t.Errorf("unexpected persisted contacts: email=%q phone=%q scheduled=%t", email, phone, scheduled)
				}
			} else {
				var after string
				if err := pool.QueryRow(ctx, `SELECT row_to_json(a)::text FROM appointments a WHERE id = $1`, appointmentID).Scan(&after); err != nil {
					t.Fatal(err)
				}
				if before != after {
					t.Error("rejected request modified the appointment")
				}
				if tc.status == http.StatusNotFound && !strings.Contains(response.Body.String(), `"Appointment not found"`) {
					t.Errorf("unexpected 404 body: %s", response.Body.String())
				}
			}
		})
	}
}

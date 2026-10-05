//go:build integration

package server

import (
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
			for _, file := range []string{"003_appointments.sql", "020_appointments_soft_delete.sql"} {
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
			// Body identity/practice fields must never override authenticated/path scope.
			body := `{"is_scheduled":true,"is_cancelled":false,"scheduled_date":"2026-10-06T00:00:00Z","scheduled_time":"09:00:00","id":"` + uuid.NewString() + `","practice_id":"` + practiceID.String() + `"}`
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
				if _, err := pool.Exec(ctx, `ALTER TABLE appointments ADD CONSTRAINT injected_failure CHECK (NOT is_scheduled)`); err != nil {
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
				var appointment Appointment
				if err := json.Unmarshal(response.Body.Bytes(), &appointment); err != nil {
					t.Fatal(err)
				}
				if appointment.ID != appointmentID || appointment.PracticeID != practiceID || appointment.IsScheduled == nil || !*appointment.IsScheduled {
					t.Fatalf("unexpected appointment response: %+v", appointment)
				}
				var saved bool
				if err := pool.QueryRow(ctx, `SELECT is_scheduled AND NOT is_cancelled AND scheduled_date = '2026-10-06' AND scheduled_time = '09:00:00'
					FROM appointments WHERE id = $1`, appointmentID).Scan(&saved); err != nil {
					t.Fatal(err)
				}
				if !saved {
					t.Error("schedule was not saved")
				}
			} else {
				var after string
				if err := pool.QueryRow(ctx, `SELECT row_to_json(a)::text FROM appointments a WHERE id = $1`, appointmentID).Scan(&after); err != nil {
					t.Fatal(err)
				}
				if before != after {
					t.Error("rejected request modified the appointment")
				}
				if tc.status == http.StatusNotFound && response.Body.String() != `{"error":"Appointment not found"}` {
					t.Errorf("unexpected 404 body: %s", response.Body.String())
				}
			}
		})
	}
}

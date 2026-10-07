//go:build integration

package server

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestAppointmentDetailDeleteScope(t *testing.T) {
	config := registrationTestConfig(t)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				status int
			}{
				{"own practice", http.StatusOK},
				{"other practice", http.StatusNotFound},
				{"missing appointment", http.StatusNotFound},
				{"invalid ID", http.StatusBadRequest},
				{"missing practice", http.StatusForbidden},
				{"database error", http.StatusInternalServerError},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					t.Cleanup(cancel)
					pool := newRegistrationTestPool(t, ctx, config)
					practiceID, otherPracticeID, appointmentID := seedAppointmentScope(t, ctx, pool)
					before := appointmentScopeSnapshot(t, ctx, pool, nil)
					unrelatedBefore := appointmentScopeSnapshot(t, ctx, pool, &otherPracticeID)
					user := AuthUser{PracticeId: &practiceID}
					path := "/appointments/" + appointmentID.String()
					switch tc.name {
					case "other practice":
						user.PracticeId = &otherPracticeID
					case "missing appointment":
						path = "/appointments/" + uuid.NewString()
					case "invalid ID":
						path = "/appointments/not-a-uuid"
					case "missing practice":
						user.PracticeId = nil
					case "database error":
						if _, err := pool.Exec(ctx, `ALTER TABLE appointments RENAME TO unavailable_appointments`); err != nil {
							t.Fatal(err)
						}
					}
					// A supplied practice ID cannot override the authenticated membership.
					path += "?practice_id=" + practiceID.String()
					s := &Server{DBQuery: db.New(pool)}
					router := gin.New()
					router.Handle(method, "/appointments/:id", func(c *gin.Context) {
						c.Set("user", user)
						if method == http.MethodGet {
							s.handlerGetAppointmentById(c)
						} else {
							s.handlerAppointmentsDelete(c)
						}
					})
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest(method, path, nil).WithContext(ctx))
					if tc.name == "database error" {
						if _, err := pool.Exec(ctx, `ALTER TABLE unavailable_appointments RENAME TO appointments`); err != nil {
							t.Fatal(err)
						}
					}
					if response.Code != tc.status {
						t.Fatalf("status %d, want %d: %s", response.Code, tc.status, response.Body.String())
					}
					if tc.status == http.StatusOK && method == http.MethodDelete {
						var message string
						if err := json.Unmarshal(response.Body.Bytes(), &message); err != nil {
							t.Fatal(err)
						}
						if want := fmt.Sprintf("Successfully deleted appointment with id: %v", appointmentID); message != want {
							t.Errorf("message %q, want %q", message, want)
						}
						var exists bool
						if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM appointments WHERE id = $1)`, appointmentID).Scan(&exists); err != nil {
							t.Fatal(err)
						}
						if exists {
							t.Error("authorized appointment was not deleted")
						}
					} else if after := appointmentScopeSnapshot(t, ctx, pool, nil); before != after {
						t.Error("read or rejected request changed appointment data")
					}
					if after := appointmentScopeSnapshot(t, ctx, pool, &otherPracticeID); unrelatedBefore != after {
						t.Error("other practice's appointment changed")
					}
					if tc.status == http.StatusOK && method == http.MethodGet {
						var appointment Appointment
						if err := json.Unmarshal(response.Body.Bytes(), &appointment); err != nil {
							t.Fatal(err)
						}
						if appointment.ID != appointmentID || appointment.PracticeID != practiceID || appointment.FirstName != "Test" {
							t.Errorf("unexpected appointment: %+v", appointment)
						}
					}
					if tc.status == http.StatusNotFound && response.Body.String() != `{"error":"Appointment not found"}` {
						t.Errorf("unexpected 404 body: %s", response.Body.String())
					}
				})
			}
		})
	}
}

func seedAppointmentScope(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
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
	for _, fixture := range []struct{ practice, appointment uuid.UUID }{
		{practiceID, appointmentID}, {otherPracticeID, uuid.New()},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, 'Fixture', 'Test City', $2, 'dental')`, fixture.practice, fixture.practice.String()); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO appointments
			(id, practice_id, first_name, last_name, email, mobile_phone, requested_date, token)
			VALUES ($1, $2, 'Test', 'Patient', 'fixture@example.test', '+15555550100', '2026-10-01', $3)`,
			fixture.appointment, fixture.practice, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
	}
	return practiceID, otherPracticeID, appointmentID
}

func appointmentScopeSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, practiceID *uuid.UUID) string {
	t.Helper()
	var snapshot string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(json_agg(a ORDER BY id), '[]'::json)::text
		FROM appointments a WHERE $1::uuid IS NULL OR practice_id = $1`, practiceID).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

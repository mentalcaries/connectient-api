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

func TestPatientsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyPatientUniquenessMigration(t, ctx, pool)
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	for _, practice := range []struct {
		id, name, code string
	}{
		{practiceID.String(), "Patient Practice", "patient-practice"},
		{otherPracticeID.String(), "Other Patient Practice", "other-patient-practice"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, $2, 'Test City', $3, 'dental')`, practice.id, practice.name, practice.code); err != nil {
			t.Fatal(err)
		}
	}
	patientID, duplicateID, otherPatientID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO patients
		(id, practice_id, first_name, last_name, email, mobile_phone, home_phone, date_of_birth, notes, created_at)
		VALUES
		($1, $4, 'Jane', 'Doe', 'jane@example.test', '+15555550100', '+15555550109', '1990-04-05', 'Original', NOW() + INTERVAL '2 hours'),
		($2, $4, 'Duplicate', 'Patient', 'duplicate@example.test', '+15555550101', NULL, NULL, NULL, NOW() + INTERVAL '1 hour'),
		($3, $5, 'Other', 'Tenant', 'jane@example.test', '+15555550100', NULL, NULL, NULL, NOW())`,
		patientID, duplicateID, otherPatientID, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO patients (practice_id, first_name, last_name, created_at)
		SELECT $1, 'Searchable', 'Patient ' || value::text, NOW() - (value || ' minutes')::interval
		FROM generate_series(1, 21) AS value`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO patients (practice_id, first_name, last_name, created_at)
		SELECT $1, 'Bulk', 'Patient ' || value::text, NOW() - (value || ' days')::interval
		FROM generate_series(1, 500) AS value`, practiceID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	user := AuthUser{PracticeId: &practiceID}

	response := requestPatients(ctx, s, http.MethodGet, "/patients", user, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("patient list = %d %s", response.Code, response.Body.String())
	}
	var list struct {
		Patients []PatientSummaryResponse `json:"patients"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Patients) != 500 || list.Patients[0].ID != patientID || list.Patients[0].DateOfBirth == nil || *list.Patients[0].DateOfBirth != "1990-04-05" {
		t.Errorf("unexpected unfiltered list: count=%d first=%+v", len(list.Patients), list.Patients[0])
	}
	response = requestPatients(ctx, s, http.MethodGet, "/patients?q=Searchable%25_,()", user, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("patient search = %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Patients) != 20 {
		t.Errorf("search count = %d, want 20", len(list.Patients))
	}

	response = requestPatients(ctx, s, http.MethodGet, "/patients/"+patientID.String(), user, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"date_of_birth":"1990-04-05"`) {
		t.Fatalf("patient detail = %d %s", response.Code, response.Body.String())
	}
	response = requestPatients(ctx, s, http.MethodGet, "/patients/"+otherPatientID.String(), user, nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant detail = %d, want 404", response.Code)
	}

	response = requestPatients(ctx, s, http.MethodPatch, "/patients/"+patientID.String(), user, map[string]any{
		"first_name": " Updated ", "email": "updated@example.test", "home_phone": "",
		"date_of_birth": "1991-05-06", "email_consent": false, "notes": "Updated notes",
		"id": otherPatientID, "practice_id": otherPracticeID, "created_at": time.Time{}, "force": true,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("patient patch = %d %s", response.Code, response.Body.String())
	}
	var persisted struct {
		PracticeID   uuid.UUID
		FirstName    string
		Email        *string
		HomePhone    *string
		DateOfBirth  *time.Time
		EmailConsent bool
		Notes        *string
	}
	if err := pool.QueryRow(ctx, `SELECT practice_id, first_name, email, home_phone, date_of_birth, email_consent, notes
		FROM patients WHERE id = $1`, patientID).Scan(
		&persisted.PracticeID, &persisted.FirstName, &persisted.Email, &persisted.HomePhone,
		&persisted.DateOfBirth, &persisted.EmailConsent, &persisted.Notes,
	); err != nil {
		t.Fatal(err)
	}
	if persisted.PracticeID != practiceID || persisted.FirstName != "Updated" || persisted.Email == nil ||
		*persisted.Email != "updated@example.test" || persisted.HomePhone != nil || persisted.DateOfBirth == nil ||
		persisted.DateOfBirth.Format("2006-01-02") != "1991-05-06" || persisted.EmailConsent ||
		persisted.Notes == nil || *persisted.Notes != "Updated notes" {
		t.Errorf("unexpected persistence: %+v", persisted)
	}

	response = requestPatients(ctx, s, http.MethodPatch, "/patients/"+patientID.String(), user, map[string]any{"email": "duplicate@example.test"})
	if response.Code != http.StatusConflict || response.Body.String() != `{"error":"duplicate_email"}` {
		t.Errorf("duplicate email = %d %s", response.Code, response.Body.String())
	}
	response = requestPatients(ctx, s, http.MethodPatch, "/patients/"+patientID.String(), user, map[string]any{"mobile_phone": "+15555550101"})
	if response.Code != http.StatusConflict || response.Body.String() != `{"error":"duplicate_phone"}` {
		t.Errorf("duplicate phone = %d %s", response.Code, response.Body.String())
	}
	response = requestPatients(ctx, s, http.MethodPatch, "/patients/"+patientID.String(), user, map[string]any{
		"email": "jane@example.test", "mobile_phone": "+15555550100",
	})
	if response.Code != http.StatusOK {
		t.Errorf("cross-practice contacts should be valid: %d %s", response.Code, response.Body.String())
	}
	response = requestPatients(ctx, s, http.MethodPatch, "/patients/"+otherPatientID.String(), user, map[string]any{"notes": "attack"})
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant patch = %d, want 404", response.Code)
	}
	var otherNotes *string
	if err := pool.QueryRow(ctx, `SELECT notes FROM patients WHERE id = $1`, otherPatientID).Scan(&otherNotes); err != nil || otherNotes != nil {
		t.Errorf("cross-tenant patient changed: notes=%v err=%v", otherNotes, err)
	}
}

func applyPatientUniquenessMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, file := range []string{"008_patients.sql", "026_patient_contact_uniqueness.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(data), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatal(err)
		}
	}
}

func requestPatients(ctx context.Context, s *Server, method, path string, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/patients", setUser, s.handlerListPatients)
	router.GET("/patients/:id", setUser, s.handlerGetPatient)
	router.PATCH("/patients/:id", setUser, s.handlerPatchPatient)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

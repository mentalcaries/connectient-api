//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestProcedureTypesIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyTestSchemas(t, ctx, pool, "024_procedure_type_integrity.sql")

	practiceID, otherPracticeID := uuid.New(), uuid.New()
	for id, code := range map[uuid.UUID]string{practiceID: "procedure-practice", otherPracticeID: "other-procedure-practice"} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, 'Procedure Practice', 'Test City', $2, 'dental')`, id, code); err != nil {
			t.Fatal(err)
		}
	}
	defaultID, inactiveID, deletedID := uuid.New(), uuid.New(), uuid.New()
	for _, fixture := range []struct {
		id                           uuid.UUID
		name, value                  string
		order                        int
		active, defaultType, primary bool
		deleted                      bool
	}{
		{defaultID, "Default", "default", 1, true, true, true, false},
		{inactiveID, "Inactive", "inactive", 2, false, false, false, false},
		{deletedID, "Deleted", "deleted-value", 9, true, false, false, true},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO procedure_types
			(id, practice_id, name, value, sort_order, is_active, is_default, is_primary, deleted_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CASE WHEN $9 THEN NOW() END)`,
			fixture.id, practiceID, fixture.name, fixture.value, fixture.order,
			fixture.active, fixture.defaultType, fixture.primary, fixture.deleted); err != nil {
			t.Fatal(err)
		}
	}
	otherID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO procedure_types (id, practice_id, name, value, sort_order)
		VALUES ($1, $2, 'Other tenant', 'other', 1)`, otherID, otherPracticeID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	owner, admin, staff := "owner", "admin", "staff"
	response := requestProcedureTypes(ctx, s, http.MethodGet, "/practices/procedure-types", AuthUser{PracticeId: &practiceID, Role: &staff}, nil, false)
	var list struct {
		Success bool            `json:"success"`
		Data    []ProcedureType `json:"data"`
	}
	decodeResponse(t, response, &list)
	if response.Code != http.StatusOK || !list.Success || len(list.Data) != 2 || list.Data[0].ID != defaultID || list.Data[1].ID != inactiveID || list.Data[1].IsActive {
		t.Fatalf("unexpected procedure list: status=%d data=%+v", response.Code, list)
	}

	response = requestProcedureTypes(ctx, s, http.MethodPost, "/practices/procedure-types", AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{
		"name": "New primary", "value": "new-primary", "is_primary": true,
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}
	var created struct {
		Success bool          `json:"success"`
		Data    ProcedureType `json:"data"`
	}
	decodeResponse(t, response, &created)
	if !created.Success || created.Data.SortOrder != 10 || !created.Data.IsPrimary || created.Data.IsDefault || !created.Data.IsActive {
		t.Errorf("unexpected created procedure: %+v", created.Data)
	}
	assertPrimaryProcedure(t, ctx, pool, practiceID, created.Data.ID)

	response = requestProcedureTypes(ctx, s, http.MethodPost, "/practices/procedure-types", AuthUser{PracticeId: &practiceID, Role: &admin}, map[string]any{
		"name": "Duplicate", "value": "deleted-value",
	}, true)
	if response.Code != http.StatusConflict {
		t.Errorf("duplicate status = %d, want 409: %s", response.Code, response.Body.String())
	}
	response = requestProcedureTypes(ctx, s, http.MethodPost, "/practices/procedure-types", AuthUser{PracticeId: &practiceID, Role: &staff}, map[string]any{
		"name": "Denied", "value": "denied",
	}, true)
	if response.Code != http.StatusForbidden {
		t.Errorf("staff mutation status = %d, want 403", response.Code)
	}

	response = requestProcedureTypes(ctx, s, http.MethodPatch, "/practices/procedure-types/"+defaultID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"name": "Renamed default"}, true)
	if response.Code != http.StatusForbidden {
		t.Errorf("default rename status = %d, want 403", response.Code)
	}
	response = requestProcedureTypes(ctx, s, http.MethodPatch, "/practices/procedure-types/"+defaultID.String(), AuthUser{PracticeId: &practiceID, Role: &admin}, map[string]any{"is_active": false}, true)
	if response.Code != http.StatusOK {
		t.Errorf("default active update = %d %s", response.Code, response.Body.String())
	}
	response = requestProcedureTypes(ctx, s, http.MethodPatch, "/practices/procedure-types/"+otherID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"name": "Cross tenant"}, true)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", response.Code)
	}

	response = requestProcedureTypes(ctx, s, http.MethodDelete, "/practices/procedure-types/"+defaultID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, nil, true)
	if response.Code != http.StatusForbidden {
		t.Errorf("default delete status = %d, want 403", response.Code)
	}
	response = requestProcedureTypes(ctx, s, http.MethodDelete, "/practices/procedure-types/"+created.Data.ID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, nil, true)
	if response.Code != http.StatusOK {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}
	response = requestProcedureTypes(ctx, s, http.MethodGet, "/practices/procedure-types", AuthUser{PracticeId: &practiceID, Role: &staff}, nil, false)
	decodeResponse(t, response, &list)
	if len(list.Data) != 2 {
		t.Errorf("soft-deleted procedure remained in list: %+v", list.Data)
	}
}

func requestProcedureTypes(ctx context.Context, s *Server, method, path string, user AuthUser, body any, adminOnly bool) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	handlers := []gin.HandlerFunc{func(c *gin.Context) { c.Set("user", user) }}
	if adminOnly {
		handlers = append(handlers, requireAdmin())
	}
	switch method {
	case http.MethodGet:
		handlers = append(handlers, s.handlerGetPracticeProcedures)
		router.GET(path, handlers...)
	case http.MethodPost:
		handlers = append(handlers, s.handlerCreateProcedureType)
		router.POST(path, handlers...)
	case http.MethodPatch:
		handlers = append(handlers, s.handlerPatchProcedureType)
		router.PATCH("/practices/procedure-types/:id", handlers...)
	case http.MethodDelete:
		handlers = append(handlers, s.handlerDeleteProcedureType)
		router.DELETE("/practices/procedure-types/:id", handlers...)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func assertPrimaryProcedure(t *testing.T, ctx context.Context, pool *pgxpool.Pool, practiceID, wantID uuid.UUID) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT id FROM procedure_types WHERE practice_id = $1 AND is_primary`, practiceID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 1 || ids[0] != wantID {
		t.Errorf("primary procedures = %v, want [%s]", ids, wantID)
	}
}

//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestProvidersIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyTestSchemas(t, ctx, pool, "006_provider.sql", "009_practice_provider.sql", "025_provider_ownership.sql")

	practiceID, otherPracticeID, emptyPracticeID, failurePracticeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for id, code := range map[uuid.UUID]string{
		practiceID: "provider-practice", otherPracticeID: "other-provider-practice",
		emptyPracticeID: "empty-provider-practice", failurePracticeID: "failure-provider-practice",
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, 'Provider Practice', 'Test City', $2, 'dental')`, id, code); err != nil {
			t.Fatal(err)
		}
	}
	mainID := insertProviderFixture(t, ctx, pool, practiceID, "Main", true)
	additionalID := insertProviderFixture(t, ctx, pool, practiceID, "Additional", false)
	otherID := insertProviderFixture(t, ctx, pool, otherPracticeID, "Other", true)

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	owner, admin, staff := "owner", "admin", "staff"
	firstResponse := requestProviders(ctx, s, http.MethodPost, "/practices/providers", AuthUser{PracticeId: &emptyPracticeID, Role: &owner}, map[string]any{
		"first_name": "First", "last_name": "Provider", "is_main": false,
	}, true)
	var firstCreated struct {
		Data ProviderResponse `json:"data"`
	}
	decodeResponse(t, firstResponse, &firstCreated)
	if firstResponse.Code != http.StatusCreated || !firstCreated.Data.IsMain {
		t.Fatalf("first provider should become main: status=%d data=%+v", firstResponse.Code, firstCreated.Data)
	}
	assertMainProvider(t, ctx, pool, emptyPracticeID, firstCreated.Data.ID)

	response := requestProviders(ctx, s, http.MethodGet, "/practices/providers", AuthUser{PracticeId: &practiceID, Role: &staff}, nil, false)
	var list struct {
		Data []ProviderResponse `json:"data"`
	}
	decodeResponse(t, response, &list)
	if response.Code != http.StatusOK || len(list.Data) != 2 || list.Data[0].ID != mainID || !list.Data[0].IsMain || list.Data[1].ID != additionalID {
		t.Fatalf("unexpected provider list: status=%d data=%+v", response.Code, list.Data)
	}

	response = requestProviders(ctx, s, http.MethodPost, "/practices/providers", AuthUser{PracticeId: &practiceID, Role: &admin}, map[string]any{
		"first_name": " New ", "last_name": " Provider ",
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}
	var created struct {
		Data ProviderResponse `json:"data"`
	}
	decodeResponse(t, response, &created)
	if created.Data.FirstName != "New" || created.Data.LastName != "Provider" || created.Data.Specialty != "General" || created.Data.IsMain {
		t.Errorf("unexpected created provider: %+v", created.Data)
	}
	response = requestProviders(ctx, s, http.MethodPost, "/practices/providers", AuthUser{PracticeId: &practiceID, Role: &staff}, map[string]any{
		"first_name": "Denied", "last_name": "Provider",
	}, true)
	if response.Code != http.StatusForbidden {
		t.Errorf("staff create status = %d, want 403", response.Code)
	}

	response = requestProviders(ctx, s, http.MethodPatch, "/practices/providers/"+created.Data.ID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{
		"first_name": "Updated", "title": "Dr", "specialty": "Dental",
	}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("patch response = %d %s", response.Code, response.Body.String())
	}
	var updated struct {
		Data ProviderResponse `json:"data"`
	}
	decodeResponse(t, response, &updated)
	if updated.Data.FirstName != "Updated" || updated.Data.Title == nil || *updated.Data.Title != "Dr" || updated.Data.Specialty != "Dental" {
		t.Errorf("unexpected updated provider: %+v", updated.Data)
	}
	response = requestProviders(ctx, s, http.MethodPatch, "/practices/providers/"+otherID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"first_name": "Cross tenant"}, true)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant patch status = %d, want 404", response.Code)
	}

	response = requestProviders(ctx, s, http.MethodPut, "/practices/providers/"+created.Data.ID.String()+"/main", AuthUser{PracticeId: &practiceID, Role: &owner}, nil, true)
	if response.Code != http.StatusOK {
		t.Fatalf("set-main response = %d %s", response.Code, response.Body.String())
	}
	assertMainProvider(t, ctx, pool, practiceID, created.Data.ID)
	response = requestProviders(ctx, s, http.MethodDelete, "/practices/providers/"+created.Data.ID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, nil, true)
	if response.Code != http.StatusBadRequest {
		t.Errorf("main delete status = %d, want 400", response.Code)
	}
	response = requestProviders(ctx, s, http.MethodDelete, "/practices/providers/"+additionalID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, nil, true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("unlink response = %d %s", response.Code, response.Body.String())
	}
	var providerExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM provider WHERE id = $1)`, additionalID).Scan(&providerExists); err != nil || !providerExists {
		t.Errorf("provider row should remain: exists=%t err=%v", providerExists, err)
	}

	assertProviderCreateRollback(t, ctx, pool, s, failurePracticeID, owner)
}

func insertProviderFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, practiceID uuid.UUID, firstName string, main bool) uuid.UUID {
	t.Helper()
	providerID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO provider (id, first_name, last_name) VALUES ($1, $2, 'Fixture')`, providerID, firstName); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_provider (practice_id, provider_id, is_main) VALUES ($1, $2, $3)`, practiceID, providerID, main); err != nil {
		t.Fatal(err)
	}
	return providerID
}

func requestProviders(ctx context.Context, s *Server, method, path string, user AuthUser, body any, adminOnly bool) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	handlers := []gin.HandlerFunc{func(c *gin.Context) { c.Set("user", user) }}
	if adminOnly {
		handlers = append(handlers, requireAdmin())
	}
	switch method {
	case http.MethodGet:
		handlers = append(handlers, s.handlerGetPracticeProviders)
		router.GET(path, handlers...)
	case http.MethodPost:
		handlers = append(handlers, s.handlerCreatePracticeProvider)
		router.POST(path, handlers...)
	case http.MethodPatch:
		handlers = append(handlers, s.handlerPatchPracticeProvider)
		router.PATCH("/practices/providers/:id", handlers...)
	case http.MethodPut:
		handlers = append(handlers, s.handlerSetMainPracticeProvider)
		router.PUT("/practices/providers/:id/main", handlers...)
	case http.MethodDelete:
		handlers = append(handlers, s.handlerDeletePracticeProvider)
		router.DELETE("/practices/providers/:id", handlers...)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func assertMainProvider(t *testing.T, ctx context.Context, pool *pgxpool.Pool, practiceID, wantID uuid.UUID) {
	t.Helper()
	var gotID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT provider_id FROM practice_provider WHERE practice_id = $1 AND is_main`, practiceID).Scan(&gotID); err != nil {
		t.Fatal(err)
	}
	if gotID != wantID {
		t.Errorf("main provider = %s, want %s", gotID, wantID)
	}
}

func assertProviderCreateRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool, s *Server, practiceID uuid.UUID, owner string) {
	t.Helper()
	constraint := fmt.Sprintf(`ALTER TABLE practice_provider ADD CONSTRAINT injected_provider_failure CHECK (practice_id <> '%s'::uuid)`, practiceID)
	if _, err := pool.Exec(ctx, constraint); err != nil {
		t.Fatal(err)
	}
	response := requestProviders(ctx, s, http.MethodPost, "/practices/providers", AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{
		"first_name": "Rollback", "last_name": "Provider",
	}, true)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("rollback response = %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM provider WHERE first_name = 'Rollback' AND last_name = 'Provider'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("provider insert was not rolled back: count=%d", count)
	}
}

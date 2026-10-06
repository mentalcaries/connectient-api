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

func TestPracticeLocationsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyTestSchemas(t, ctx, pool, "007_practice_locations.sql", "022_available_weekdays.sql", "023_location_address_nullable.sql")

	practiceID, otherPracticeID := uuid.New(), uuid.New()
	for id, code := range map[uuid.UUID]string{practiceID: "location-practice", otherPracticeID: "other-location-practice"} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, 'Location Practice', 'Test City', $2, 'dental')`, id, code); err != nil {
			t.Fatal(err)
		}
	}
	activeID, inactiveID := uuid.New(), uuid.New()
	for _, fixture := range []struct {
		id      uuid.UUID
		name    string
		order   int
		active  bool
		deleted bool
	}{
		{activeID, "Second", 2, true, false},
		{inactiveID, "First inactive", 1, false, false},
		{uuid.New(), "Deleted maximum", 8, true, true},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO practice_locations
			(id, practice_id, name, address, is_active, sort_order, deleted_at)
			VALUES ($1, $2, $3, 'Fixture address', $4, $5, CASE WHEN $6 THEN NOW() END)`,
			fixture.id, practiceID, fixture.name, fixture.active, fixture.order, fixture.deleted); err != nil {
			t.Fatal(err)
		}
	}
	otherLocationID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practice_locations (id, practice_id, name, sort_order)
		VALUES ($1, $2, 'Other tenant', 1)`, otherLocationID, otherPracticeID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	owner, admin, staff := "owner", "admin", "staff"
	response := requestPracticeLocations(ctx, s, http.MethodGet, "/practices/locations", AuthUser{PracticeId: &practiceID, Role: &staff}, nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}
	var list struct {
		Success bool                       `json:"success"`
		Data    []PracticeSettingsLocation `json:"data"`
	}
	decodeResponse(t, response, &list)
	if !list.Success || len(list.Data) != 2 || list.Data[0].ID != inactiveID || list.Data[1].ID != activeID || list.Data[0].IsActive {
		t.Fatalf("unexpected location list: %+v", list)
	}

	response = requestPracticeLocations(ctx, s, http.MethodPost, "/practices/locations", AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"name": "New location"}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", response.Code, response.Body.String())
	}
	var created struct {
		Success bool                     `json:"success"`
		Data    PracticeSettingsLocation `json:"data"`
	}
	decodeResponse(t, response, &created)
	if !created.Success || created.Data.Address != nil || created.Data.SortOrder != 9 || !created.Data.IsActive || len(created.Data.AvailableWeekdays) != 6 {
		t.Errorf("unexpected created location: %+v", created.Data)
	}

	response = requestPracticeLocations(ctx, s, http.MethodPatch, "/practices/locations/"+created.Data.ID.String(), AuthUser{PracticeId: &practiceID, Role: &admin}, map[string]any{
		"address": nil, "is_active": false, "sort_order": 4, "available_weekdays": []int{5, 2, 2},
	}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("patch response = %d %s", response.Code, response.Body.String())
	}
	assertPatchedLocation(t, ctx, pool, created.Data.ID)

	response = requestPracticeLocations(ctx, s, http.MethodPatch, "/practices/locations/"+otherLocationID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"name": "Cross tenant"}, true)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant status = %d, want 404", response.Code)
	}
	response = requestPracticeLocations(ctx, s, http.MethodPost, "/practices/locations", AuthUser{PracticeId: &practiceID, Role: &staff}, map[string]any{"name": "Denied"}, true)
	if response.Code != http.StatusForbidden {
		t.Errorf("staff mutation status = %d, want 403", response.Code)
	}

	response = requestPracticeLocations(ctx, s, http.MethodPatch, "/practices/locations/"+created.Data.ID.String(), AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"deleted_at": time.Now().UTC().Format(time.RFC3339)}, true)
	if response.Code != http.StatusOK {
		t.Fatalf("soft-delete response = %d %s", response.Code, response.Body.String())
	}
	response = requestPracticeLocations(ctx, s, http.MethodGet, "/practices/locations", AuthUser{PracticeId: &practiceID, Role: &staff}, nil, false)
	decodeResponse(t, response, &list)
	if len(list.Data) != 2 {
		t.Errorf("soft-deleted location remained in list: %+v", list.Data)
	}
}

func requestPracticeLocations(ctx context.Context, s *Server, method, path string, user AuthUser, body any, adminOnly bool) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	handlers := []gin.HandlerFunc{func(c *gin.Context) { c.Set("user", user) }}
	if adminOnly {
		handlers = append(handlers, requireAdmin())
	}
	switch method {
	case http.MethodGet:
		handlers = append(handlers, s.handlerGetPracticeLocations)
		router.GET(path, handlers...)
	case http.MethodPost:
		handlers = append(handlers, s.handlerCreatePracticeLocation)
		router.POST(path, handlers...)
	case http.MethodPatch:
		handlers = append(handlers, s.handlerPatchPracticeLocation)
		router.PATCH("/practices/locations/:id", handlers...)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func assertPatchedLocation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, locationID uuid.UUID) {
	t.Helper()
	var address *string
	var active bool
	var order int32
	var weekdays []int16
	if err := pool.QueryRow(ctx, `SELECT address, is_active, sort_order, available_weekdays FROM practice_locations WHERE id = $1`, locationID).
		Scan(&address, &active, &order, &weekdays); err != nil {
		t.Fatal(err)
	}
	if address != nil || active || order != 4 || len(weekdays) != 2 || weekdays[0] != 2 || weekdays[1] != 5 {
		t.Errorf("unexpected patched location: address=%v active=%t order=%d weekdays=%v", address, active, order, weekdays)
	}
}

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
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestOnboardingProgressIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", "030_onboarding_progress.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}

	practiceID, userID, otherUserID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Progress Practice', 'Test City', 'progress-practice', 'dental')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Progress', 'User', 'progress@example.test', $3, 'owner'),
		       ($2, 'Other', 'User', 'other-progress@example.test', $3, 'staff')`, userID, otherUserID, practiceID); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}

	response := requestOnboardingProgress(ctx, s, userID, http.MethodGet, "/onboarding/progress", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"setup":null`) ||
		!strings.Contains(response.Body.String(), `"create-appointment":null`) {
		t.Fatalf("initial progress = %d %s", response.Code, response.Body.String())
	}
	response = requestOnboardingProgress(ctx, s, userID, http.MethodPatch, "/onboarding/progress/setup", map[string]any{"seen": true})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"walkthrough_key":"setup"`) ||
		strings.Contains(response.Body.String(), `"seen_at":null`) {
		t.Fatalf("seen update = %d %s", response.Code, response.Body.String())
	}
	var seenAt time.Time
	if err := pool.QueryRow(ctx, `SELECT seen_at FROM onboarding_progress WHERE user_id = $1 AND walkthrough_key = 'setup'`, userID).Scan(&seenAt); err != nil {
		t.Fatal(err)
	}
	response = requestOnboardingProgress(ctx, s, userID, http.MethodPatch, "/onboarding/progress/setup", map[string]any{"completed": true})
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"completed_at":null`) {
		t.Fatalf("completed update = %d %s", response.Code, response.Body.String())
	}
	var storedSeenAt time.Time
	if err := pool.QueryRow(ctx, `SELECT seen_at FROM onboarding_progress WHERE user_id = $1 AND walkthrough_key = 'setup'`, userID).Scan(&storedSeenAt); err != nil || !storedSeenAt.Equal(seenAt) {
		t.Errorf("seen timestamp changed: before=%s after=%s err=%v", seenAt, storedSeenAt, err)
	}
	response = requestOnboardingProgress(ctx, s, otherUserID, http.MethodGet, "/onboarding/progress", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"setup":null`) {
		t.Errorf("other user progress = %d %s", response.Code, response.Body.String())
	}
	for _, tc := range []struct {
		path string
		body any
	}{
		{"/onboarding/progress/unknown", map[string]any{"seen": true}},
		{"/onboarding/progress/setup", map[string]any{"seen": false}},
	} {
		response = requestOnboardingProgress(ctx, s, userID, http.MethodPatch, tc.path, tc.body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid update %s = %d %s", tc.path, response.Code, response.Body.String())
		}
	}
}

func TestOnboardingPracticeCodesIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	if _, err := pool.Exec(ctx, `INSERT INTO practices (name, city, practice_code, practice_category)
		VALUES ('Taken', 'Test City', 'my-practice-name', 'dental'),
		       ('Taken 2', 'Test City', 'my-practice-name-2', 'dental')`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	claims := TokenClaims{ID: uuid.New(), Email: "codes@example.test"}

	response := requestOnboardingProgress(ctx, s, claims.ID, http.MethodGet, "/onboarding/check-code?code=My%20--%20Practice!%20Name", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) ||
		!strings.Contains(response.Body.String(), `"code":"my-practice-name"`) {
		t.Fatalf("check code = %d %s", response.Code, response.Body.String())
	}
	response = requestOnboardingProgress(ctx, s, claims.ID, http.MethodGet, "/onboarding/check-code?code=ADMIN", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) {
		t.Fatalf("reserved code = %d %s", response.Code, response.Body.String())
	}
	response = requestOnboardingProgress(ctx, s, claims.ID, http.MethodGet, "/onboarding/suggest-code?name=My%20Practice%20Name%20Limited", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"suggestion":"my-practice-name-3"`) {
		t.Fatalf("suggest code = %d %s", response.Code, response.Body.String())
	}
}

func requestOnboardingProgress(ctx context.Context, s *Server, userID uuid.UUID, method, path string, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setClaims := func(c *gin.Context) { c.Set("claims", TokenClaims{ID: userID, Email: "progress@example.test"}) }
	router.GET("/onboarding/check-code", setClaims, s.handlerCheckCodeAvailability)
	router.GET("/onboarding/suggest-code", setClaims, s.handlerSuggestPracticeCode)
	router.GET("/onboarding/progress", setClaims, s.handlerGetOnboardingProgress)
	router.PATCH("/onboarding/progress/:key", setClaims, s.handlerPatchOnboardingProgress)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestInternalMembershipLookup(t *testing.T) {
	ctx := context.Background()
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	defer pool.Close()
	t.Setenv("AUTH_PROFILE_SERVICE_TOKEN", "fixture-service-token")

	practiceID, userID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, email, practice_code, practice_category) VALUES ($1, 'Lookup Practice', 'Test', 'lookup@example.test', $2, 'dental')`, practiceID, "lookup-"+practiceID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, first_name, last_name, practice_id, role, is_active) VALUES ($1, 'owner@example.test', 'Test', 'Owner', $2, 'owner', TRUE)`, userID, practiceID); err != nil {
		t.Fatal(err)
	}

	s := &Server{DBQuery: db.New(pool)}
	router := gin.New()
	router.POST("/internal/auth/membership", s.handlerInternalMembershipLookup)
	request := httptest.NewRequest(http.MethodPost, "/internal/auth/membership", strings.NewReader(`{"userId":"`+userID.String()+`"}`))
	request.Header.Set("Authorization", "Bearer fixture-service-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), practiceID.String()) || !strings.Contains(response.Body.String(), `"role":"owner"`) {
		t.Fatalf("lookup = %d %s", response.Code, response.Body.String())
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET is_active = FALSE WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/internal/auth/membership", strings.NewReader(`{"userId":"`+userID.String()+`"}`))
	request.Header.Set("Authorization", "Bearer fixture-service-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("inactive lookup = %d %s", response.Code, response.Body.String())
	}

	t.Setenv("E2E_TEST_MODE", "true")
	router.DELETE("/internal/test/memberships/:id", s.handlerDeleteTestMembership)
	request = httptest.NewRequest(http.MethodDelete, "/internal/test/memberships/"+userID.String(), nil)
	request.Header.Set("Authorization", "Bearer fixture-service-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete fixture = %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, userID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("membership remains count=%d err=%v", count, err)
	}
}

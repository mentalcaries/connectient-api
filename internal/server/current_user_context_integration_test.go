//go:build integration

package server

import (
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

func TestCurrentUserContextIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	identity, practiceID := uuid.New(), uuid.New()
	s := &Server{DBQuery: db.New(pool)}

	response := requestCurrentUser(t, ctx, s, identity, "identity@example.test", "/me/context")
	if response.Code != http.StatusOK {
		t.Fatalf("unonboarded status = %d", response.Code)
	}
	var unonboarded CurrentUserContext
	decodeResponse(t, response, &unonboarded)
	if !unonboarded.OnboardingRequired || unonboarded.Membership != nil || unonboarded.Practice != nil || unonboarded.Subscription != nil || len(unonboarded.Permissions) != 0 {
		t.Fatalf("unexpected unonboarded context: %+v", unonboarded)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, practice_code, practice_category, email, has_multiple_providers)
		VALUES ($1, 'Context Practice', 'Test City', 'context-practice', 'dental', 'practice@example.test', true)`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users
		(id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Context', 'Owner', 'member@example.test', $2, 'owner')`, identity, practiceID); err != nil {
		t.Fatal(err)
	}

	response = requestCurrentUser(t, ctx, s, identity, "identity@example.test", "/me/context")
	var missingSubscription CurrentUserContext
	decodeResponse(t, response, &missingSubscription)
	if response.Code != http.StatusOK || missingSubscription.OnboardingRequired || missingSubscription.Subscription == nil || missingSubscription.Subscription.Status != "none" {
		t.Fatalf("unexpected missing-subscription context: %+v", missingSubscription)
	}
	if got := missingSubscription.Permissions; len(got) != 1 || got[0] != "billing:manage" {
		t.Fatalf("recovery permissions = %v", got)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status)
		VALUES ('pro', $1, 'active')`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	response = requestCurrentUser(t, ctx, s, identity, "identity@example.test", "/me/context")
	var onboarded CurrentUserContext
	decodeResponse(t, response, &onboarded)
	if response.Code != http.StatusOK || onboarded.Identity.Email != "identity@example.test" || onboarded.Practice == nil ||
		onboarded.Practice.Name != "Context Practice" || onboarded.Membership == nil || onboarded.Membership.AccessRevoked ||
		onboarded.Subscription == nil || !onboarded.Subscription.IsActive || len(onboarded.Permissions) != 7 {
		t.Fatalf("unexpected onboarded context: %+v", onboarded)
	}

	// The legacy endpoint must preserve absent subscription dates as JSON null.
	response = requestCurrentUser(t, ctx, s, identity, "identity@example.test", "/users/me")
	var legacy map[string]any
	decodeResponse(t, response, &legacy)
	for _, field := range []string{"trial_end", "period_end", "cancel_at"} {
		if legacy[field] != nil {
			t.Errorf("%s = %v, want null", field, legacy[field])
		}
	}

	if _, err := pool.Exec(ctx, `UPDATE practices SET is_suspended = true WHERE id = $1`, practiceID); err != nil {
		t.Fatal(err)
	}
	response = requestCurrentUser(t, ctx, s, identity, "identity@example.test", "/me/context")
	var suspended CurrentUserContext
	decodeResponse(t, response, &suspended)
	if suspended.Membership == nil || !suspended.Membership.AccessRevoked || len(suspended.Permissions) != 0 {
		t.Fatalf("suspended context must be restricted: %+v", suspended)
	}

	assertCurrentUserDatabaseFailure(t, ctx, pool, s, identity)
}

func requestCurrentUser(t *testing.T, ctx context.Context, s *Server, identity uuid.UUID, email, path string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	handler := s.handlerGetCurrentUserContext
	if path == "/users/me" {
		handler = s.handlerGetCurrentUser
	}
	router.GET(path, func(c *gin.Context) {
		c.Set("claims", TokenClaims{ID: identity, Email: email})
		handler(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
}

func assertCurrentUserDatabaseFailure(t *testing.T, ctx context.Context, pool *pgxpool.Pool, s *Server, identity uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx, `ALTER TABLE users RENAME TO unavailable_users`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/me/context", "/users/me"} {
		response := requestCurrentUser(t, ctx, s, identity, "identity@example.test", path)
		if response.Code != http.StatusInternalServerError {
			t.Errorf("%s status = %d, want 500", path, response.Code)
		}
	}
}

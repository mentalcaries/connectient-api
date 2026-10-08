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

func TestTeamMembersIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", "011_practice_invites.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	ownerID, adminID, staffID, inactiveID, otherID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Team Practice', 'Test City', 'team-practice', 'dental'),
		       ($2, 'Other Team', 'Test City', 'other-team', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users
		(id, first_name, last_name, email, practice_id, role, is_active, created_at)
		VALUES
		($1, 'Owner', 'User', 'owner@example.test', $6, 'owner', TRUE, NOW() - INTERVAL '5 days'),
		($2, 'Admin', 'User', 'admin@example.test', $6, 'admin', TRUE, NOW() - INTERVAL '4 days'),
		($3, 'Staff', 'User', 'staff@example.test', $6, 'staff', FALSE, NOW() - INTERVAL '3 days'),
		($4, 'Inactive', 'User', 'inactive@example.test', $6, 'staff', FALSE, NOW() - INTERVAL '2 days'),
		($5, 'Other', 'User', 'other@example.test', $7, 'staff', TRUE, NOW())`,
		ownerID, adminID, staffID, inactiveID, otherID, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status) VALUES ('pro', $1, 'active')`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	inviteID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practice_invites
		(id, practice_id, email, first_name, last_name, role, invited_by, token, token_expires_at)
		VALUES ($1, $2, 'invite@example.test', 'Invite', 'User', 'staff', $3, $4, NOW() + INTERVAL '1 day')`,
		inviteID, practiceID, ownerID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	ownerRole := "owner"
	caller := AuthUser{ID: ownerID, PracticeId: &practiceID, Role: &ownerRole}

	response := requestTeam(ctx, s, http.MethodGet, "/users", caller, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"is_owner":true`) || strings.Contains(response.Body.String(), otherID.String()) {
		t.Fatalf("user list = %d %s", response.Code, response.Body.String())
	}
	if strings.Index(response.Body.String(), ownerID.String()) > strings.Index(response.Body.String(), adminID.String()) {
		t.Errorf("users not oldest first: %s", response.Body.String())
	}

	response = requestTeam(ctx, s, http.MethodGet, "/users/seats", caller, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"used":3`) || !strings.Contains(response.Body.String(), `"atLimit":true`) {
		t.Fatalf("seat usage = %d %s", response.Code, response.Body.String())
	}
	response = requestTeam(ctx, s, http.MethodPatch, "/users/"+inactiveID.String(), caller, map[string]any{"is_active": true})
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"seat_limit_reached"`) {
		t.Fatalf("at-limit reactivation = %d %s", response.Code, response.Body.String())
	}
	if _, err := pool.Exec(ctx, `DELETE FROM practice_invites WHERE id = $1`, inviteID); err != nil {
		t.Fatal(err)
	}
	response = requestTeam(ctx, s, http.MethodPatch, "/users/"+inactiveID.String(), caller, map[string]any{"is_active": true, "role": "admin"})
	if response.Code != http.StatusOK {
		t.Fatalf("reactivation = %d %s", response.Code, response.Body.String())
	}
	var active bool
	var role *string
	if err := pool.QueryRow(ctx, `SELECT is_active, role FROM users WHERE id = $1`, inactiveID).Scan(&active, &role); err != nil || !active || role == nil || *role != "admin" {
		t.Errorf("reactivation not persisted: active=%t role=%v err=%v", active, role, err)
	}

	for _, tc := range []struct {
		path string
		body map[string]any
	}{
		{"/users/" + ownerID.String(), map[string]any{"role": "staff"}},
		{"/users/" + adminID.String(), map[string]any{"role": "staff"}},
	} {
		actor := caller
		if strings.Contains(tc.path, adminID.String()) {
			actor.ID = adminID
		}
		response = requestTeam(ctx, s, http.MethodPatch, tc.path, actor, tc.body)
		if response.Code != http.StatusForbidden {
			t.Errorf("protected patch %s = %d", tc.path, response.Code)
		}
	}
	response = requestTeam(ctx, s, http.MethodPatch, "/users/"+otherID.String(), caller, map[string]any{"role": "admin"})
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant patch = %d", response.Code)
	}
	response = requestTeam(ctx, s, http.MethodDelete, "/users/"+staffID.String(), caller, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("delete staff = %d %s", response.Code, response.Body.String())
	}
	var deletedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT is_active, deleted_at FROM users WHERE id = $1`, staffID).Scan(&active, &deletedAt); err != nil || active || deletedAt == nil {
		t.Errorf("staff not soft deleted: active=%t deleted=%v err=%v", active, deletedAt, err)
	}
}

func requestTeam(ctx context.Context, s *Server, method, path string, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/users", setUser, requireAdmin(), s.handlerListPracticeUsers)
	router.GET("/users/seats", setUser, requireAdmin(), s.handlerGetPracticeSeats)
	router.PATCH("/users/:id", setUser, requireAdmin(), s.handlerPatchPracticeUser)
	router.DELETE("/users/:id", setUser, requireAdmin(), s.handlerDeletePracticeUser)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

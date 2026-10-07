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

type fakeTeamInviteNotifier struct {
	calls int
	last  TeamInviteNotification
}

func (f *fakeTeamInviteNotifier) SendTeamInvite(_ context.Context, notification TeamInviteNotification) (RegistrationDeliveryResult, error) {
	f.calls++
	f.last = notification
	return RegistrationDeliverySimulated, nil
}

func TestInvitationsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	for _, file := range []string{"011_practice_invites.sql", "029_practice_invite_email_unique.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatal(err)
		}
	}
	practiceID, otherPracticeID, ownerID, adminID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Invite Practice', 'Test City', 'invite-practice', 'dental'),
		       ($2, 'Other Invite', 'Test City', 'other-invite', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Owner', 'User', 'owner@example.test', $3, 'owner'),
		       ($2, 'Admin', 'User', 'admin@example.test', $3, 'admin')`, ownerID, adminID, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status) VALUES ('pro', $1, 'active')`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	notifier := &fakeTeamInviteNotifier{}
	s := &Server{
		db: registrationTestDB{pool}, DBQuery: db.New(pool), teamInviteNotify: notifier,
		inviteBaseURL: "https://app.example.test",
	}
	role := "owner"
	caller := AuthUser{ID: ownerID, PracticeId: &practiceID, Role: &role, Name: "Owner User"}

	response := requestInvite(ctx, s, http.MethodPost, "/users/invites", caller, map[string]any{
		"first_name": "Jane", "last_name": "Doe", "email": "JANE@EXAMPLE.TEST", "role": "staff", "org_role": "Nurse",
	})
	if response.Code != http.StatusCreated || notifier.calls != 1 || notifier.last.PracticeName != "Invite Practice" || !strings.Contains(notifier.last.Link, "/invite?token=") {
		t.Fatalf("create invite = %d %s notifier=%+v", response.Code, response.Body.String(), notifier)
	}
	var inviteID uuid.UUID
	var token string
	if err := pool.QueryRow(ctx, `SELECT id, token FROM practice_invites WHERE practice_id = $1 AND email = 'jane@example.test'`, practiceID).Scan(&inviteID, &token); err != nil {
		t.Fatal(err)
	}
	response = requestInvite(ctx, s, http.MethodGet, "/users/invites", caller, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"email":"jane@example.test"`) || strings.Contains(response.Body.String(), token) {
		t.Fatalf("invite list = %d %s", response.Code, response.Body.String())
	}
	response = requestInvite(ctx, s, http.MethodPost, "/users/invites", caller, map[string]any{
		"first_name": "Another", "last_name": "Invite", "email": "another@example.test", "role": "staff",
	})
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "seat_limit_reached") {
		t.Errorf("seat limit = %d %s", response.Code, response.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE subscription SET plan = 'practice' WHERE "referenceId" = $1`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	response = requestInvite(ctx, s, http.MethodPost, "/users/invites", caller, map[string]any{
		"first_name": "Existing", "last_name": "Admin", "email": "ADMIN@example.test", "role": "staff",
	})
	if response.Code != http.StatusConflict {
		t.Errorf("existing member invite = %d %s", response.Code, response.Body.String())
	}

	oldToken := token
	response = requestInvite(ctx, s, http.MethodPost, "/users/invites/"+inviteID.String()+"/resend", caller, nil)
	if response.Code != http.StatusOK || notifier.calls != 2 {
		t.Fatalf("resend = %d %s calls=%d", response.Code, response.Body.String(), notifier.calls)
	}
	if err := pool.QueryRow(ctx, `SELECT token FROM practice_invites WHERE id = $1`, inviteID).Scan(&token); err != nil || token == oldToken {
		t.Errorf("token not rotated: old=%q new=%q err=%v", oldToken, token, err)
	}
	response = requestInvite(ctx, s, http.MethodGet, "/invite/validate?token="+token, caller, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"valid":true`) || !strings.Contains(response.Body.String(), `"practice_name":"Invite Practice"`) {
		t.Fatalf("validate = %d %s", response.Code, response.Body.String())
	}
	if _, err := pool.Exec(ctx, `UPDATE practice_invites SET token_expires_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, inviteID); err != nil {
		t.Fatal(err)
	}
	response = requestInvite(ctx, s, http.MethodGet, "/invite/validate?token="+token, caller, nil)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Invite expired") {
		t.Errorf("expired validation = %d %s", response.Code, response.Body.String())
	}
	response = requestInvite(ctx, s, http.MethodDelete, "/users/invites/"+inviteID.String(), caller, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("delete invite = %d %s", response.Code, response.Body.String())
	}
	response = requestInvite(ctx, s, http.MethodDelete, "/users/invites/"+inviteID.String(), caller, nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("missing delete = %d", response.Code)
	}

	otherInviteID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practice_invites
		(id, practice_id, email, first_name, last_name, role, invited_by, token, token_expires_at)
		VALUES ($1, $2, 'other@example.test', 'Other', 'Invite', 'staff', $3, $4, $5)`,
		otherInviteID, otherPracticeID, ownerID, uuid.NewString(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	response = requestInvite(ctx, s, http.MethodDelete, "/users/invites/"+otherInviteID.String(), caller, nil)
	if response.Code != http.StatusNotFound {
		t.Errorf("cross-tenant delete = %d", response.Code)
	}
}

func requestInvite(ctx context.Context, s *Server, method, path string, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	router.GET("/users/invites", setUser, requireAdmin(), s.handlerListPracticeInvites)
	router.POST("/users/invites", setUser, requireAdmin(), s.handlerCreatePracticeInvite)
	router.DELETE("/users/invites/:id", setUser, requireAdmin(), s.handlerDeletePracticeInvite)
	router.POST("/users/invites/:id/resend", setUser, requireAdmin(), s.handlerResendPracticeInvite)
	router.GET("/invite/validate", s.handlerValidateInvite)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

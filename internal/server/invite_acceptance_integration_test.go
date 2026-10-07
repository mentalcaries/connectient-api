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
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type fakeIdentityProfileService struct {
	mu        sync.Mutex
	calls     int
	failCalls int
	lastID    uuid.UUID
	lastName  string
}

func (f *fakeIdentityProfileService) UpdateDisplayName(_ context.Context, id uuid.UUID, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastID, f.lastName = id, name
	if f.calls <= f.failCalls {
		return errFakeIdentitySync
	}
	return nil
}

func TestInviteAcceptanceIntegration(t *testing.T) {
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

	practiceID, otherPracticeID, ownerID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Acceptance Practice', 'Test City', 'acceptance-practice', 'dental'),
		       ($2, 'Other Practice', 'Test City', 'other-acceptance', 'dental')`, practiceID, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
		VALUES ($1, 'Owner', 'User', 'owner@example.test', $2, 'owner')`, ownerID, practiceID); err != nil {
		t.Fatal(err)
	}
	identity := uuid.New()
	notifier := &fakeIdentityProfileService{failCalls: 1}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool), identityProfiles: notifier}
	token := insertAcceptanceInvite(t, ctx, pool, practiceID, ownerID, "Invited@Example.Test", time.Now().Add(time.Hour))
	body := map[string]any{
		"token": token, "first_name": "Jane", "last_name": "Doe",
		"mobile_phone": "+15551234567", "termsAgreed": true,
	}

	response := requestInviteAcceptance(ctx, s, TokenClaims{ID: identity, Email: "invited@example.test"}, body)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"identity_sync_failed"`) ||
		!strings.Contains(response.Body.String(), `"membership_created":true`) {
		t.Fatalf("first acceptance = %d %s", response.Code, response.Body.String())
	}
	assertAcceptedMembership(t, ctx, pool, identity, practiceID, token)

	if _, err := pool.Exec(ctx, `UPDATE practice_invites SET token_expires_at = NOW() - INTERVAL '1 hour' WHERE token = $1`, token); err != nil {
		t.Fatal(err)
	}
	body["first_name"], body["last_name"] = "Changed", "Name"
	response = requestInviteAcceptance(ctx, s, TokenClaims{ID: identity, Email: "INVITED@example.test"}, body)
	if response.Code != http.StatusOK {
		t.Fatalf("idempotent sync retry = %d %s", response.Code, response.Body.String())
	}
	if notifier.calls != 2 || notifier.lastID != identity || notifier.lastName != "Jane Doe" {
		t.Errorf("identity calls = %+v", notifier)
	}

	t.Run("pending invite does not consume existing membership", func(t *testing.T) {
		existingIdentity := uuid.New()
		existingEmail := "existing@example.test"
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
			VALUES ($1, 'Existing', 'Member', $2, $3, 'staff')`, existingIdentity, existingEmail, practiceID); err != nil {
			t.Fatal(err)
		}
		existingToken := insertAcceptanceInvite(t, ctx, pool, practiceID, ownerID, existingEmail, time.Now().Add(time.Hour))
		response := requestInviteAcceptance(ctx, s, TokenClaims{ID: existingIdentity, Email: existingEmail}, acceptanceBody(existingToken))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"membership_conflict"`) {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		assertInvitePending(t, ctx, pool, existingToken)
	})
	var membershipCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, identity).Scan(&membershipCount); err != nil || membershipCount != 1 {
		t.Errorf("membership count=%d err=%v", membershipCount, err)
	}

	t.Run("email mismatch leaves invite pending", func(t *testing.T) {
		mismatchToken := insertAcceptanceInvite(t, ctx, pool, practiceID, ownerID, "right@example.test", time.Now().Add(time.Hour))
		response := requestInviteAcceptance(ctx, s, TokenClaims{ID: uuid.New(), Email: "wrong@example.test"}, acceptanceBody(mismatchToken))
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"email_mismatch"`) {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		assertInvitePending(t, ctx, pool, mismatchToken)
	})

	t.Run("expired invite leaves no membership", func(t *testing.T) {
		expiredIdentity := uuid.New()
		expiredToken := insertAcceptanceInvite(t, ctx, pool, practiceID, ownerID, "expired@example.test", time.Now().Add(-time.Hour))
		response := requestInviteAcceptance(ctx, s, TokenClaims{ID: expiredIdentity, Email: "expired@example.test"}, acceptanceBody(expiredToken))
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Invite expired") {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		assertNoMembership(t, ctx, pool, expiredIdentity)
		assertInvitePending(t, ctx, pool, expiredToken)
	})

	t.Run("different practice membership conflicts", func(t *testing.T) {
		conflictIdentity := uuid.New()
		conflictEmail := "conflict@example.test"
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role)
			VALUES ($1, 'Existing', 'User', $2, $3, 'staff')`, conflictIdentity, conflictEmail, otherPracticeID); err != nil {
			t.Fatal(err)
		}
		conflictToken := insertAcceptanceInvite(t, ctx, pool, practiceID, ownerID, conflictEmail, time.Now().Add(time.Hour))
		response := requestInviteAcceptance(ctx, s, TokenClaims{ID: conflictIdentity, Email: conflictEmail}, acceptanceBody(conflictToken))
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"membership_conflict"`) {
			t.Fatalf("response = %d %s", response.Code, response.Body.String())
		}
		assertInvitePending(t, ctx, pool, conflictToken)
	})

	t.Run("concurrent retries create one membership", func(t *testing.T) {
		concurrentIdentity := uuid.New()
		concurrentEmail := "concurrent@example.test"
		concurrentToken := insertAcceptanceInvite(t, ctx, pool, practiceID, ownerID, concurrentEmail, time.Now().Add(time.Hour))
		concurrentBody := acceptanceBody(concurrentToken)
		start := make(chan struct{})
		codes := make(chan int, 2)
		for range 2 {
			go func() {
				<-start
				codes <- requestInviteAcceptance(ctx, s, TokenClaims{ID: concurrentIdentity, Email: concurrentEmail}, concurrentBody).Code
			}()
		}
		close(start)
		for range 2 {
			if code := <-codes; code != http.StatusOK {
				t.Errorf("concurrent status = %d", code)
			}
		}
		assertAcceptedMembership(t, ctx, pool, concurrentIdentity, practiceID, concurrentToken)
	})
}

func insertAcceptanceInvite(t *testing.T, ctx context.Context, pool db.DBTX, practiceID, ownerID uuid.UUID, email string, expiresAt time.Time) string {
	t.Helper()
	token := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO practice_invites
		(practice_id, email, first_name, last_name, role, invited_by, token, token_expires_at)
		VALUES ($1, $2, 'Invited', 'User', 'staff', $3, $4, $5)`, practiceID, email, ownerID, token, expiresAt); err != nil {
		t.Fatal(err)
	}
	return token
}

func acceptanceBody(token string) map[string]any {
	return map[string]any{"token": token, "first_name": "Invited", "last_name": "User", "termsAgreed": true}
}

func requestInviteAcceptance(ctx context.Context, s *Server, claims TokenClaims, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	router.POST("/invite/accept", func(c *gin.Context) { c.Set("claims", claims) }, s.handlerAcceptInvite)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/invite/accept", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func assertAcceptedMembership(t *testing.T, ctx context.Context, pool db.DBTX, identity, practiceID uuid.UUID, token string) {
	t.Helper()
	var storedPractice uuid.UUID
	var termsAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT practice_id, terms_agreed_at FROM users WHERE id = $1`, identity).Scan(&storedPractice, &termsAt); err != nil {
		t.Fatal(err)
	}
	if storedPractice != practiceID || termsAt == nil {
		t.Errorf("membership practice=%s terms=%v", storedPractice, termsAt)
	}
	var acceptedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT accepted_at FROM practice_invites WHERE token = $1`, token).Scan(&acceptedAt); err != nil || acceptedAt == nil {
		t.Errorf("invite accepted_at=%v err=%v", acceptedAt, err)
	}
}

func assertInvitePending(t *testing.T, ctx context.Context, pool db.DBTX, token string) {
	t.Helper()
	var acceptedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT accepted_at FROM practice_invites WHERE token = $1`, token).Scan(&acceptedAt); err != nil || acceptedAt != nil {
		t.Errorf("invite accepted_at=%v err=%v", acceptedAt, err)
	}
}

func assertNoMembership(t *testing.T, ctx context.Context, pool db.DBTX, identity uuid.UUID) {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, identity).Scan(&count); err != nil || count != 0 {
		t.Errorf("membership count=%d err=%v", count, err)
	}
}

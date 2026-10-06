//go:build integration

package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestAuthorizationQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	userID, practiceID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Fixture', 'Test City', 'auth-fixture', 'dental')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, practice_id, role)
		VALUES ($1, 'Test', 'Member', $2, 'staff')`, userID, practiceID); err != nil {
		t.Fatal(err)
	}
	queries := db.New(pool)

	user, err := queries.GetUserAuthorization(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != userID || user.PracticeID == nil || *user.PracticeID != practiceID || user.Role == nil || *user.Role != "staff" || !user.IsActive || user.IsSuspended {
		t.Fatalf("unexpected membership: %+v", user)
	}

	// The next lookup must reflect a practice suspension, not stale membership.
	if _, err := pool.Exec(ctx, `UPDATE practices SET is_suspended = true WHERE id = $1`, practiceID); err != nil {
		t.Fatal(err)
	}
	user, err = queries.GetUserAuthorization(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsSuspended {
		t.Error("query did not load the current practice suspension")
	}

	// An identity without a joined practice is not an authorized membership.
	if _, err := pool.Exec(ctx, `UPDATE users SET practice_id = NULL WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetUserAuthorization(ctx, userID); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unassigned user: got %v, want ErrNoRows", err)
	}
	if _, err := queries.GetUserAuthorization(ctx, uuid.New()); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unknown user: got %v, want ErrNoRows", err)
	}
}

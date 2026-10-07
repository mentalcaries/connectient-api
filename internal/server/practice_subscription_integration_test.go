//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestPracticeSubscriptionIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	for _, practice := range []struct {
		id   uuid.UUID
		name string
		code string
	}{
		{practiceID, "Subscription Practice", "subscription-practice"},
		{otherPracticeID, "Other Practice", "other-subscription-practice"},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
			VALUES ($1, $2, 'Test City', $3, 'dental')`, practice.id, practice.name, practice.code); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription
		(plan, "referenceId", status, "periodEnd", "stripeCustomerId")
		VALUES ('practice', $1, 'active', $2, 'cus_private'),
		       ('essential', $3, 'active', $2, 'cus_other_private')`,
		practiceID.String(), time.Now().Add(24*time.Hour), otherPracticeID.String()); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	user := AuthUser{PracticeId: &practiceID}

	response := requestPracticeSubscription(ctx, s, user)
	if response.Code != http.StatusOK {
		t.Fatalf("active subscription = %d %s", response.Code, response.Body.String())
	}
	assertSubscriptionResponse(t, response, "active", true, false, false, true)
	for _, forbidden := range []string{"stripe", "cus_private", "cus_other_private", "essential"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("subscription response exposed %q: %s", forbidden, response.Body.String())
		}
	}

	if _, err := pool.Exec(ctx, `UPDATE subscription SET status = 'past_due', "periodEnd" = $2 WHERE "referenceId" = $1`,
		practiceID.String(), time.Now().Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	response = requestPracticeSubscription(ctx, s, user)
	assertSubscriptionResponse(t, response, "past_due", false, true, true, true)

	if _, err := pool.Exec(ctx, `UPDATE subscription SET "periodEnd" = $2 WHERE "referenceId" = $1`,
		practiceID.String(), time.Now().Add(-31*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	response = requestPracticeSubscription(ctx, s, user)
	assertSubscriptionResponse(t, response, "past_due", false, true, false, false)

	if _, err := pool.Exec(ctx, `DELETE FROM subscription WHERE "referenceId" = $1`, practiceID.String()); err != nil {
		t.Fatal(err)
	}
	response = requestPracticeSubscription(ctx, s, user)
	assertSubscriptionResponse(t, response, "none", false, false, false, false)
	var decoded struct {
		Subscription SubscriptionContext `json:"subscription"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Subscription.Plan != nil || decoded.Subscription.ExpiryDate != nil || decoded.Subscription.GracePeriodEndsAt != nil {
		t.Errorf("missing subscription retained values: %+v", decoded.Subscription)
	}
}

func requestPracticeSubscription(ctx context.Context, s *Server, user AuthUser) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/practices/subscription", func(c *gin.Context) { c.Set("user", user) }, s.handlerGetPracticeSubscription)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/practices/subscription", bytes.NewReader(nil)).WithContext(ctx)
	router.ServeHTTP(response, request)
	return response
}

func assertSubscriptionResponse(t *testing.T, response *httptest.ResponseRecorder, status string, active, expired, grace, access bool) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("subscription response = %d %s", response.Code, response.Body.String())
	}
	var decoded struct {
		Subscription SubscriptionContext `json:"subscription"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	got := decoded.Subscription
	if got.Status != status || got.IsActive != active || got.IsExpired != expired ||
		got.IsInGracePeriod != grace || got.CanAccessBookings != access ||
		got.CanAccessRegistrations != access || got.CanAccessCalendar != access {
		t.Errorf("unexpected subscription: %+v", got)
	}
}

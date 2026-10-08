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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type registrationTestDB struct{ pool *pgxpool.Pool }

func (d registrationTestDB) Pool() *pgxpool.Pool     { return d.pool }
func (d registrationTestDB) Close() error            { d.pool.Close(); return nil }
func (d registrationTestDB) Health() db.HealthStatus { return db.HealthStatus{} }

func TestOnboardingTransaction(t *testing.T) {
	config := registrationTestConfig(t)

	for _, tc := range []struct {
		name       string
		code       string
		failureSQL string
		status     int
	}{
		{name: "commit", code: "transaction-fixture", status: http.StatusCreated},
		{name: "reserved code rollback", code: "admin", status: http.StatusConflict},
		{name: "procedure failure rollback", code: "transaction-fixture", status: http.StatusInternalServerError,
			failureSQL: `ALTER TABLE procedure_types ADD CONSTRAINT injected_failure CHECK (value <> 'cleaning')`},
		{name: "subscription failure rollback", code: "transaction-fixture", status: http.StatusInternalServerError,
			failureSQL: `ALTER TABLE subscription ADD CONSTRAINT injected_failure CHECK (plan <> 'pro')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			t.Cleanup(cancel)
			pool := newOnboardingTestPool(t, ctx, config)
			if tc.failureSQL != "" {
				if _, err := pool.Exec(ctx, tc.failureSQL); err != nil {
					t.Fatalf("inject database failure: %v", err)
				}
			}

			identity := uuid.New()
			response := serveOnboarding(t, ctx, pool, identity, tc.code)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
			assertOnboardingCounts(t, ctx, pool, tc.status == http.StatusCreated)
			if tc.status == http.StatusCreated {
				assertOnboardingCommitted(t, ctx, pool, identity, response.Body.Bytes())
			}
		})
	}
}

func TestOnboardingValidationAndConflicts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newOnboardingTestPool(t, ctx, registrationTestConfig(t))
	identity := uuid.New()
	body := onboardingFixtureBody("contract-practice", "+15555550101")
	body["termsAgreed"] = false
	response := serveOnboardingBody(t, ctx, pool, identity, body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "terms_required") {
		t.Fatalf("terms response = %d %s", response.Code, response.Body.String())
	}
	assertOnboardingCounts(t, ctx, pool, false)

	body["termsAgreed"] = true
	response = serveOnboardingBody(t, ctx, pool, identity, body)
	if response.Code != http.StatusCreated {
		t.Fatalf("success response = %d %s", response.Code, response.Body.String())
	}
	response = serveOnboardingBody(t, ctx, pool, identity, body)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Onboarding already completed") {
		t.Fatalf("existing identity = %d %s", response.Code, response.Body.String())
	}

	response = serveOnboardingBody(t, ctx, pool, uuid.New(), onboardingFixtureBody("contract-practice", "+15555550102"))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Practice code is already taken") {
		t.Fatalf("duplicate code = %d %s", response.Code, response.Body.String())
	}
}

func registrationTestConfig(t *testing.T) *pgxpool.Config {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("set TEST_DATABASE_URL to a disposable local connectient_test database (see docs/testing.md)")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "connectient_test" ||
		(config.ConnConfig.Host != "127.0.0.1" && config.ConnConfig.Host != "localhost" && config.ConnConfig.Host != "::1") {
		t.Fatal("test requires a local database named connectient_test")
	}
	return config
}

func newRegistrationTestPool(t *testing.T, ctx context.Context, config *pgxpool.Config) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	schema := "onboarding_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	// Cleanup runs in reverse order: close the test pool, drop its schema,
	// then close the admin pool. Use a fresh context after test cancellation.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	isolated := config.Copy()
	isolated.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, isolated)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, file := range []string{
		"001_practices.sql", "002_users.sql", "004_alter_practices_code_index_is_active.sql",
		"005_alter_users_deleted_at.sql", "010_practice_settings.sql", "014_procedure_types.sql", "021_subscription.sql",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		up := strings.SplitN(string(data), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
	return pool
}

func newOnboardingTestPool(t *testing.T, ctx context.Context, config *pgxpool.Config) *pgxpool.Pool {
	t.Helper()
	pool := newRegistrationTestPool(t, ctx, config)
	applyTestSchemas(t, ctx, pool, "006_provider.sql", "009_practice_provider.sql", "025_provider_ownership.sql")
	return pool
}

func serveOnboarding(t *testing.T, ctx context.Context, pool *pgxpool.Pool, identity uuid.UUID, code string) *httptest.ResponseRecorder {
	t.Helper()
	return serveOnboardingBody(t, ctx, pool, identity, onboardingFixtureBody(code, "+15555550100"))
}

func onboardingFixtureBody(code, phone string) map[string]any {
	return map[string]any{
		"is_solo_provider":  true,
		"name":              "Transaction Fixture",
		"practice_category": "dental",
		"practice_code":     code,
		"city":              "Test City",
		"first_name":        "Test",
		"last_name":         "Owner",
		"mobile_phone":      phone,
		"termsAgreed":       true,
	}
}

func serveOnboardingBody(t *testing.T, ctx context.Context, pool *pgxpool.Pool, identity uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	router := gin.New()
	router.POST("/onboarding/complete", func(c *gin.Context) {
		c.Set("claims", TokenClaims{ID: identity, Email: "fixture@example.test"})
		s.handlerCompleteOnboarding(c)
	})
	// Keep the wire keys explicit so this fixture also checks the request contract.
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode registration fixture: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/onboarding/complete", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertOnboardingCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, committed bool) {
	t.Helper()
	for _, record := range []struct {
		table string
		count int
	}{
		{"users", 1},
		{"practices", 1},
		{"practice_settings", 1},
		{"procedure_types", 5},
		{"provider", 1},
		{"practice_provider", 1},
		{"subscription", 1},
	} {
		table, expected := record.table, record.count
		if !committed {
			expected = 0
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != expected {
			t.Errorf("%s has %d rows, want %d", table, count, expected)
		}
	}
}

func assertOnboardingCommitted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, identity uuid.UUID, body []byte) {
	t.Helper()
	var payload struct {
		Success    bool      `json:"success"`
		PracticeID uuid.UUID `json:"practice_id"`
		RedirectTo string    `json:"redirect_to"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Success || payload.RedirectTo != "/admin/dashboard" {
		t.Errorf("unexpected response: %+v", payload)
	}
	var linked bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM users u JOIN practices p ON p.id = u.practice_id
		JOIN practice_settings settings ON settings.practice_id = p.id
		JOIN practice_provider pp ON pp.practice_id = p.id AND pp.is_main
		JOIN provider provider ON provider.id = pp.provider_id
		JOIN subscription s ON s."referenceId" = p.id::text
		WHERE u.id = $1 AND p.id = $2 AND u.role = 'owner' AND p.email = 'fixture@example.test'
		AND provider.first_name = u.first_name AND provider.last_name = u.last_name
		AND settings.dental_history_enabled AND s.plan = 'pro' AND s.status = 'trialing'
		AND s."trialEnd" - s."trialStart" BETWEEN interval '29 days' AND interval '31 days'
	)`, identity, payload.PracticeID).Scan(&linked)
	if err != nil {
		t.Fatal(err)
	}
	if !linked {
		t.Error("committed onboarding records are not correctly linked")
	}
}

func TestOnboardingManagerMustSupplyMainProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newOnboardingTestPool(t, ctx, registrationTestConfig(t))
	body := onboardingFixtureBody("manager-practice", "+15555550103")
	body["is_solo_provider"] = false
	body["registrant_is_provider"] = false

	response := serveOnboardingBody(t, ctx, pool, uuid.New(), body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Main provider details are required") {
		t.Fatalf("missing provider response = %d %s", response.Code, response.Body.String())
	}
	assertOnboardingCounts(t, ctx, pool, false)

	body["main_provider"] = map[string]any{
		"first_name": "Alex", "last_name": "Clinician", "title": "Dr.", "specialty": "Rheumatology",
	}
	response = serveOnboardingBody(t, ctx, pool, uuid.New(), body)
	if response.Code != http.StatusCreated {
		t.Fatalf("manager onboarding response = %d %s", response.Code, response.Body.String())
	}
	var provider struct {
		FirstName string
		LastName  string
		IsMain    bool
	}
	if err := pool.QueryRow(ctx, `SELECT p.first_name, p.last_name, pp.is_main
		FROM provider p JOIN practice_provider pp ON pp.provider_id = p.id`).Scan(
		&provider.FirstName, &provider.LastName, &provider.IsMain,
	); err != nil {
		t.Fatal(err)
	}
	if provider.FirstName != "Alex" || provider.LastName != "Clinician" || !provider.IsMain {
		t.Fatalf("unexpected manager provider: %+v", provider)
	}
}

func TestSingleProviderOwnerBackfill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newOnboardingTestPool(t, ctx, registrationTestConfig(t))

	singleID, multiID, inactiveID, existingID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, practice := range []struct {
		id       uuid.UUID
		code     string
		multiple bool
	}{
		{singleID, "single-backfill", false},
		{multiID, "multi-backfill", true},
		{inactiveID, "inactive-backfill", false},
		{existingID, "existing-backfill", false},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO practices
			(id, name, city, practice_code, practice_category, specialty, has_multiple_providers)
			VALUES ($1, 'Backfill Practice', 'Test City', $2, 'medical', 'Cardiology', $3)`,
			practice.id, practice.code, practice.multiple,
		); err != nil {
			t.Fatal(err)
		}
	}

	for _, owner := range []struct {
		id         uuid.UUID
		practiceID uuid.UUID
		active     bool
	}{
		{uuid.New(), singleID, true},
		{uuid.New(), multiID, true},
		{uuid.New(), inactiveID, false},
		{uuid.New(), existingID, true},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO users
			(id, first_name, last_name, email, practice_id, role, is_active)
			VALUES ($1, 'Practice', 'Owner', $2, $3, 'owner', $4)`,
			owner.id, owner.id.String()+"@example.test", owner.practiceID, owner.active,
		); err != nil {
			t.Fatal(err)
		}
	}

	existingProviderID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO provider (id, first_name, last_name) VALUES ($1, 'Existing', 'Provider')`, existingProviderID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_provider (practice_id, provider_id, is_main) VALUES ($1, $2, TRUE)`, existingID, existingProviderID); err != nil {
		t.Fatal(err)
	}

	applyTestSchemas(t, ctx, pool, "035_backfill_single_provider_owners.sql")

	var firstName, lastName, specialty string
	var isMain bool
	if err := pool.QueryRow(ctx, `SELECT provider.first_name, provider.last_name, provider.specialty, link.is_main
		FROM practice_provider AS link
		JOIN provider ON provider.id = link.provider_id
		WHERE link.practice_id = $1`, singleID).Scan(&firstName, &lastName, &specialty, &isMain); err != nil {
		t.Fatal(err)
	}
	if firstName != "Practice" || lastName != "Owner" || specialty != "Cardiology" || !isMain {
		t.Fatalf("unexpected backfilled provider: %s %s, %s, main=%t", firstName, lastName, specialty, isMain)
	}

	for _, practiceID := range []uuid.UUID{multiID, inactiveID} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM practice_provider WHERE practice_id = $1`, practiceID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("practice %s received %d unexpected provider links", practiceID, count)
		}
	}
	var linkedProviderID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT provider_id FROM practice_provider WHERE practice_id = $1`, existingID).Scan(&linkedProviderID); err != nil {
		t.Fatal(err)
	}
	if linkedProviderID != existingProviderID {
		t.Fatalf("existing provider link changed from %s to %s", existingProviderID, linkedProviderID)
	}
}

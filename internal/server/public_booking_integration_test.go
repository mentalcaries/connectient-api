//go:build integration

package server

import (
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
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestPublicBookingIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyPublicBookingTestSchema(t, ctx, pool)

	activeID := uuid.New()
	graceID := uuid.New()
	expiredID := uuid.New()
	missingSubscriptionID := uuid.New()
	suspendedID := uuid.New()
	inactiveID := uuid.New()
	now := time.Now().UTC()
	insertPublicPractice(t, ctx, pool, activeID, "active-booking", false, true)
	insertPublicPractice(t, ctx, pool, graceID, "grace-booking", false, false)
	insertPublicPractice(t, ctx, pool, expiredID, "expired-booking", false, false)
	insertPublicPractice(t, ctx, pool, missingSubscriptionID, "missing-subscription", false, false)
	insertPublicPractice(t, ctx, pool, suspendedID, "suspended-booking", true, false)
	insertPublicPractice(t, ctx, pool, inactiveID, "inactive-booking", false, false)
	if _, err := pool.Exec(ctx, `UPDATE practices SET is_active = FALSE WHERE id = $1`, inactiveID); err != nil {
		t.Fatal(err)
	}
	for _, subscription := range []struct {
		practiceID uuid.UUID
		status     string
		periodEnd  time.Time
	}{
		{activeID, "active", now.Add(24 * time.Hour)},
		{graceID, "past_due", now.Add(-10 * 24 * time.Hour)},
		{expiredID, "past_due", now.Add(-31 * 24 * time.Hour)},
		{suspendedID, "active", now.Add(24 * time.Hour)},
		{inactiveID, "active", now.Add(24 * time.Hour)},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO subscription (plan, "referenceId", status, "periodEnd") VALUES ('pro', $1, $2, $3)`,
			subscription.practiceID.String(), subscription.status, subscription.periodEnd); err != nil {
			t.Fatal(err)
		}
	}

	seedPublicBookingResources(t, ctx, pool, activeID)
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}

	response := requestPublicBooking(ctx, s, "/public/practices/active-booking/booking-config")
	if response.Code != http.StatusOK {
		t.Fatalf("booking config = %d %s", response.Code, response.Body.String())
	}
	var config struct {
		Practice       PublicPractice        `json:"practice"`
		Settings       PublicBookingSettings `json:"settings"`
		Providers      []PublicProvider      `json:"providers"`
		ProcedureTypes []PublicProcedureType `json:"procedure_types"`
		Locations      []PublicLocation      `json:"locations"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.Practice.ID != activeID || config.Practice.Name != "Active Booking" || config.Practice.PracticeCode != "active-booking" {
		t.Errorf("unexpected public practice: %+v", config.Practice)
	}
	if config.Settings.Theme != "custom" || len(config.Settings.AvailableWeekdays) != 3 || len(config.Providers) != 2 {
		t.Errorf("unexpected settings/providers: settings=%+v providers=%+v", config.Settings, config.Providers)
	}
	if len(config.ProcedureTypes) != 2 || config.ProcedureTypes[0].Name != "First procedure" || config.ProcedureTypes[1].Name != "Second procedure" {
		t.Errorf("procedures not filtered/sorted: %+v", config.ProcedureTypes)
	}
	if len(config.Locations) != 1 || config.Locations[0].Name != "Available location" {
		t.Errorf("locations not filtered: %+v", config.Locations)
	}
	for _, forbidden := range []string{"subscription_status", "is_suspended", "stripe", "trialEnd"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Errorf("booking config exposed %q: %s", forbidden, response.Body.String())
		}
	}

	response = requestPublicBooking(ctx, s, "/public/practices/active-booking/procedure-types")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("procedure route = %d %s", response.Code, response.Body.String())
	}
	response = requestPublicBooking(ctx, s, "/public/practices/active-booking/locations")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"multiple_locations_enabled":true`) {
		t.Fatalf("location route = %d %s", response.Code, response.Body.String())
	}

	for _, code := range []string{"grace-booking"} {
		response = requestPublicBooking(ctx, s, "/public/practices/"+code+"/booking-config")
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200: %s", code, response.Code, response.Body.String())
		}
	}
	for _, code := range []string{"expired-booking", "missing-subscription", "suspended-booking", "inactive-booking", "unknown"} {
		response = requestPublicBooking(ctx, s, "/public/practices/"+code+"/booking-config")
		if response.Code != http.StatusNotFound || response.Body.String() != `{"error":"Practice not found","success":false}` {
			t.Errorf("%s response = %d %s, want hidden 404", code, response.Code, response.Body.String())
		}
	}
}

func applyPublicBookingTestSchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, file := range []string{
		"006_provider.sql", "007_practice_locations.sql", "009_practice_provider.sql",
		"022_available_weekdays.sql", "023_location_address_nullable.sql", "025_provider_ownership.sql",
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
}

func insertPublicPractice(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, code string, suspended, multipleLocations bool) {
	t.Helper()
	name := strings.Title(strings.ReplaceAll(code, "-", " ")) //nolint:staticcheck // Test fixture only.
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, practice_code, practice_category, is_suspended, has_multiple_providers)
		VALUES ($1, $2, 'Test City', $3, 'dental', $4, TRUE)`, id, name, code, suspended); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_settings
		(practice_id, multiple_locations_enabled, available_weekdays, theme, theme_colors)
		VALUES ($1, $2, ARRAY[1,3,5]::SMALLINT[], 'custom', '{"primary":"#123456"}'::jsonb)`, id, multipleLocations); err != nil {
		t.Fatal(err)
	}
}

func seedPublicBookingResources(t *testing.T, ctx context.Context, pool *pgxpool.Pool, practiceID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO procedure_types
		(practice_id, name, value, is_active, deleted_at, sort_order, is_primary)
		VALUES
		($1, 'Second procedure', 'second', TRUE, NULL, 2, FALSE),
		($1, 'First procedure', 'first', TRUE, NULL, 1, TRUE),
		($1, 'Inactive procedure', 'inactive', FALSE, NULL, 0, FALSE),
		($1, 'Deleted procedure', 'deleted', TRUE, NOW(), 0, FALSE)`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_locations
		(practice_id, name, address, is_active, deleted_at, sort_order, available_weekdays)
		VALUES
		($1, 'Available location', NULL, TRUE, NULL, 2, ARRAY[1,2]::SMALLINT[]),
		($1, 'No weekdays', NULL, TRUE, NULL, 1, ARRAY[]::SMALLINT[]),
		($1, 'Inactive location', NULL, FALSE, NULL, 0, ARRAY[1]::SMALLINT[]),
		($1, 'Deleted location', NULL, TRUE, NOW(), 0, ARRAY[1]::SMALLINT[])`, practiceID); err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"Main", "Additional"} {
		providerID := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO provider (id, first_name, last_name, specialty) VALUES ($1, $2, 'Provider', 'general')`, providerID, name); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO practice_provider (practice_id, provider_id, is_main) VALUES ($1, $2, $3)`, practiceID, providerID, index == 0); err != nil {
			t.Fatal(err)
		}
	}
}

func requestPublicBooking(ctx context.Context, s *Server, path string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/public/practices/:code/booking-config", s.handlerGetPublicBookingConfig)
	router.GET("/public/practices/:code/procedure-types", s.handlerGetPublicProcedureTypes)
	router.GET("/public/practices/:code/locations", s.handlerGetPublicLocations)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	router.ServeHTTP(response, request)
	return response
}

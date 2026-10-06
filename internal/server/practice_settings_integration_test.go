//go:build integration

package server

import (
	"context"
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

func TestPracticeSettingsRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyTestSchemas(t, ctx, pool, "007_practice_locations.sql", "022_available_weekdays.sql")

	practiceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, practice_code, practice_category, specialty)
		VALUES ($1, 'Settings Practice', 'Test City', 'settings-practice', 'dental', 'Orthodontics')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_settings
		(practice_id, dental_history_enabled, custom_form_sections, theme_colors, available_weekdays)
		VALUES ($1, true, '{"section":"fixture"}', '{"start":"#000000"}', '{1,3,5}')`, practiceID); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, value string
		order       int
		deleted     bool
	}{
		{"Second", "second", 2, false}, {"Deleted", "deleted", 0, true}, {"First", "first", 1, false},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO procedure_types
			(practice_id, name, value, sort_order, deleted_at) VALUES ($1, $2, $3, $4, CASE WHEN $5 THEN NOW() END)`,
			practiceID, row.name, row.value, row.order, row.deleted)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		name    string
		order   int
		deleted bool
	}{
		{"Second location", 2, false}, {"Deleted location", 0, true}, {"First location", 1, false},
	} {
		_, err := pool.Exec(ctx, `INSERT INTO practice_locations
			(practice_id, name, address, sort_order, deleted_at) VALUES ($1, $2, 'Test address', $3, CASE WHEN $4 THEN NOW() END)`,
			practiceID, row.name, row.order, row.deleted)
		if err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{DBQuery: db.New(pool)}
	owner, staff := "owner", "staff"
	for _, tc := range []struct {
		role    *string
		isAdmin bool
	}{{&owner, true}, {&staff, false}} {
		response := requestPracticeSettings(ctx, s, AuthUser{PracticeId: &practiceID, Role: tc.role})
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", response.Code, response.Body.String())
		}
		var result struct {
			Success bool                 `json:"success"`
			Data    PracticeSettingsData `json:"data"`
		}
		decodeResponse(t, response, &result)
		if !result.Success || result.Data.IsAdmin != tc.isAdmin {
			t.Errorf("role %s response: %+v", *tc.role, result)
		}
		if result.Data.Practice.Name != "Settings Practice" || result.Data.Settings.Theme != "default" ||
			len(result.Data.Settings.AvailableWeekdays) != 3 || string(result.Data.Settings.CustomFormSections) != `{"section":"fixture"}` {
			t.Errorf("unexpected aggregate data: %+v", result.Data)
		}
		if len(result.Data.ProcedureTypes) != 2 || result.Data.ProcedureTypes[0].Name != "First" || result.Data.ProcedureTypes[1].Name != "Second" {
			t.Errorf("procedures not filtered/sorted: %+v", result.Data.ProcedureTypes)
		}
		if len(result.Data.Locations) != 2 || result.Data.Locations[0].Name != "First location" ||
			result.Data.Locations[1].Name != "Second location" || len(result.Data.Locations[0].AvailableWeekdays) != 6 {
			t.Errorf("locations not filtered/sorted/defaulted: %+v", result.Data.Locations)
		}
	}

	// The migration rejects weekdays outside Sunday (0) through Saturday (6).
	if _, err := pool.Exec(ctx, `UPDATE practice_settings SET available_weekdays = '{7}' WHERE practice_id = $1`, practiceID); err == nil {
		t.Error("weekday constraint accepted an invalid value")
	}

	missingSettingsPractice := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Missing Settings', 'Test City', 'missing-settings', 'dental')`, missingSettingsPractice); err != nil {
		t.Fatal(err)
	}
	response := requestPracticeSettings(ctx, s, AuthUser{PracticeId: &missingSettingsPractice, Role: &owner})
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"Failed to fetch practice settings"}` {
		t.Errorf("missing settings response = %d %s", response.Code, response.Body.String())
	}
}

func requestPracticeSettings(ctx context.Context, s *Server, user AuthUser) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/practices/settings", func(c *gin.Context) {
		c.Set("user", user)
		s.handlerGetPracticeSettings(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/practices/settings", nil).WithContext(ctx))
	return response
}

func applyTestSchemas(t *testing.T, ctx context.Context, pool *pgxpool.Pool, files ...string) {
	t.Helper()
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
}

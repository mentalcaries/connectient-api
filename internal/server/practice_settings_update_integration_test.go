//go:build integration

package server

import (
	"bytes"
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

func TestPracticeSettingsUpdateIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	applyTestSchemas(t, ctx, pool, "007_practice_locations.sql", "022_available_weekdays.sql")

	practiceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, practice_code, practice_category, specialty)
		VALUES ($1, 'Update Practice', 'Test City', 'update-practice', 'dental', 'Orthodontics')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO practice_settings
		(practice_id, custom_form_sections, theme_colors)
		VALUES ($1, '{"old":true}', '{"start":"#000000"}')`, practiceID); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool)}
	owner, admin, staff := "owner", "admin", "staff"
	colors := map[string]string{
		"start": "#d1fae5", "mid": "#eff6ff", "end": "#e0f2fe",
		"btn": "#7ab0d4", "dir": "to bottom left", "accent": "#dce6ed",
	}
	response := requestPracticeSettingsPatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{
		"specialty": "General Dentistry", "has_multiple_providers": true,
		"dental_history_enabled": true, "available_weekdays": []int{6, 1, 1, 3},
		"custom_form_sections": map[string]any{"section": "fixture"},
		"theme":                "match_logo", "theme_colors": colors,
	})
	if response.Code != http.StatusOK || response.Body.String() != `{"success":true}` {
		t.Fatalf("update response = %d %s", response.Code, response.Body.String())
	}
	assertPracticeSettingsUpdated(t, ctx, pool, practiceID)
	response = requestPracticeSettingsPatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &admin}, map[string]any{
		"tmj_history_enabled": true,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("admin update response = %d %s", response.Code, response.Body.String())
	}

	response = requestPracticeSettingsPatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{
		"specialty": nil, "custom_form_sections": nil, "theme": "minimal",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("nullable update response = %d %s", response.Code, response.Body.String())
	}
	var specialty *string
	var customSections, themeColors []byte
	var theme string
	if err := pool.QueryRow(ctx, `SELECT specialty FROM practices WHERE id = $1`, practiceID).Scan(&specialty); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT custom_form_sections, theme, theme_colors FROM practice_settings WHERE practice_id = $1`, practiceID).
		Scan(&customSections, &theme, &themeColors); err != nil {
		t.Fatal(err)
	}
	if specialty != nil || customSections != nil || theme != "minimal" || themeColors != nil {
		t.Errorf("nullable fields were not cleared: specialty=%v custom=%s theme=%s colors=%s", specialty, customSections, theme, themeColors)
	}

	response = requestPracticeSettingsPatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &staff}, map[string]any{"dental_history_enabled": false})
	if response.Code != http.StatusForbidden {
		t.Errorf("staff status = %d, want 403", response.Code)
	}
	response = requestPracticeSettingsPatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{"ignored": true})
	if response.Code != http.StatusBadRequest || response.Body.String() != `{"error":"No valid fields to update","success":false}` {
		t.Errorf("empty update response = %d %s", response.Code, response.Body.String())
	}

	assertPracticeSettingsRollback(t, ctx, pool, s, practiceID, owner)
}

func requestPracticeSettingsPatch(ctx context.Context, s *Server, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	router.PATCH("/practices/settings", func(c *gin.Context) { c.Set("user", user) }, requireAdmin(), s.handlerPatchPracticeSettings)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/practices/settings", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func assertPracticeSettingsUpdated(t *testing.T, ctx context.Context, pool *pgxpool.Pool, practiceID uuid.UUID) {
	t.Helper()
	var specialty *string
	var multipleProviders bool
	if err := pool.QueryRow(ctx, `SELECT specialty, has_multiple_providers FROM practices WHERE id = $1`, practiceID).
		Scan(&specialty, &multipleProviders); err != nil {
		t.Fatal(err)
	}
	var dental bool
	var weekdays []int16
	var customSections, themeColors []byte
	var theme string
	if err := pool.QueryRow(ctx, `SELECT dental_history_enabled, available_weekdays, custom_form_sections, theme, theme_colors
		FROM practice_settings WHERE practice_id = $1`, practiceID).
		Scan(&dental, &weekdays, &customSections, &theme, &themeColors); err != nil {
		t.Fatal(err)
	}
	if specialty == nil || *specialty != "General Dentistry" || !multipleProviders || !dental ||
		len(weekdays) != 3 || weekdays[0] != 1 || weekdays[1] != 3 || weekdays[2] != 6 ||
		string(customSections) != `{"section": "fixture"}` || theme != "match_logo" || len(themeColors) == 0 {
		t.Errorf("unexpected persisted values: specialty=%v multiple=%t dental=%t weekdays=%v custom=%s theme=%s colors=%s",
			specialty, multipleProviders, dental, weekdays, customSections, theme, themeColors)
	}
}

func assertPracticeSettingsRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool, s *Server, practiceID uuid.UUID, owner string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE practice_settings SET dental_history_enabled = false WHERE practice_id = $1`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE practice_settings ADD CONSTRAINT injected_settings_failure CHECK (NOT dental_history_enabled)`); err != nil {
		t.Fatal(err)
	}
	response := requestPracticeSettingsPatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, map[string]any{
		"specialty": "Must Roll Back", "dental_history_enabled": true,
	})
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("rollback response = %d %s", response.Code, response.Body.String())
	}
	var specialty *string
	if err := pool.QueryRow(ctx, `SELECT specialty FROM practices WHERE id = $1`, practiceID).Scan(&specialty); err != nil {
		t.Fatal(err)
	}
	if specialty != nil {
		t.Errorf("practice update was not rolled back: %q", *specialty)
	}
}

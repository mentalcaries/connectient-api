package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"

	"github.com/gin-gonic/gin"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

var (
	hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	validThemes     = []string{"default", "minimal", "ionic", "luxe", "ember", "petal", "fern", "match_logo"}
	validDirections = []string{"to bottom left", "to bottom right", "to bottom", "to top right"}
)

type themeColorsInput struct {
	Start  string `json:"start"`
	Mid    string `json:"mid"`
	End    string `json:"end"`
	Button string `json:"btn"`
	Dir    string `json:"dir"`
	Accent string `json:"accent"`
}

type practiceSettingsPatch struct {
	practice db.PatchPracticeSettingsPracticeParams
	settings db.PatchPracticeSettingsParams
}

func (patch practiceSettingsPatch) hasPracticeUpdates() bool {
	return patch.practice.SetSpecialty || patch.practice.SetHasMultipleProviders
}

func (patch practiceSettingsPatch) hasSettingsUpdates() bool {
	settings := patch.settings
	return settings.SetDentalHistoryEnabled || settings.SetTmjHistoryEnabled ||
		settings.SetMultipleLocationsEnabled || settings.SetAvailableWeekdays ||
		settings.SetCustomFormSections || settings.SetPhysiotherapyHistoryEnabled ||
		settings.SetOptometryHistoryEnabled || settings.SetTheme
}

func (s *Server) handlerPatchPracticeSettings(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondPracticeSettingsError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}

	patch, err := decodePracticeSettingsPatch(c.Request.Body)
	if err != nil {
		respondPracticeSettingsError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !patch.hasPracticeUpdates() && !patch.hasSettingsUpdates() {
		respondPracticeSettingsError(c, http.StatusBadRequest, "No valid fields to update", nil)
		return
	}
	patch.practice.ID = *user.PracticeId
	patch.settings.PracticeID = *user.PracticeId

	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondPracticeSettingsError(c, http.StatusInternalServerError, "Failed to update settings", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)

	if patch.hasPracticeUpdates() {
		rows, err := queries.PatchPracticeSettingsPractice(c, patch.practice)
		if err != nil || rows != 1 {
			if err == nil {
				err = errors.New("practice not found")
			}
			respondPracticeSettingsError(c, http.StatusInternalServerError, "Failed to update practice", err)
			return
		}
	}
	if patch.hasSettingsUpdates() {
		rows, err := queries.PatchPracticeSettings(c, patch.settings)
		if err != nil || rows != 1 {
			if err == nil {
				err = errors.New("practice settings not found")
			}
			respondPracticeSettingsError(c, http.StatusInternalServerError, "Failed to update settings", err)
			return
		}
	}
	if err := tx.Commit(c); err != nil {
		respondPracticeSettingsError(c, http.StatusInternalServerError, "Failed to update settings", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodePracticeSettingsPatch(body io.Reader) (practiceSettingsPatch, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return practiceSettingsPatch{}, errors.New("Invalid request")
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return practiceSettingsPatch{}, errors.New("Invalid request")
	}

	var patch practiceSettingsPatch
	var err error
	if raw, ok := fields["specialty"]; ok {
		patch.practice.SetSpecialty = true
		if isJSONNull(raw) {
			patch.practice.Specialty = nil
		} else {
			var specialty string
			if err := json.Unmarshal(raw, &specialty); err != nil {
				return practiceSettingsPatch{}, errors.New("Invalid specialty")
			}
			patch.practice.Specialty = &specialty
		}
	}
	if raw, ok := fields["has_multiple_providers"]; ok {
		patch.practice.HasMultipleProviders, err = decodeBoolean(raw, "has_multiple_providers")
		if err != nil {
			return practiceSettingsPatch{}, err
		}
		patch.practice.SetHasMultipleProviders = true
	}

	booleanSettings := []struct {
		name  string
		set   *bool
		value *bool
	}{
		{"dental_history_enabled", &patch.settings.SetDentalHistoryEnabled, &patch.settings.DentalHistoryEnabled},
		{"tmj_history_enabled", &patch.settings.SetTmjHistoryEnabled, &patch.settings.TmjHistoryEnabled},
		{"multiple_locations_enabled", &patch.settings.SetMultipleLocationsEnabled, &patch.settings.MultipleLocationsEnabled},
		{"physiotherapy_history_enabled", &patch.settings.SetPhysiotherapyHistoryEnabled, &patch.settings.PhysiotherapyHistoryEnabled},
		{"optometry_history_enabled", &patch.settings.SetOptometryHistoryEnabled, &patch.settings.OptometryHistoryEnabled},
	}
	for _, field := range booleanSettings {
		raw, ok := fields[field.name]
		if !ok {
			continue
		}
		*field.value, err = decodeBoolean(raw, field.name)
		if err != nil {
			return practiceSettingsPatch{}, err
		}
		*field.set = true
	}

	if raw, ok := fields["available_weekdays"]; ok {
		weekdays, err := normalizeAvailableWeekdays(raw)
		if err != nil {
			return practiceSettingsPatch{}, err
		}
		patch.settings.SetAvailableWeekdays = true
		patch.settings.AvailableWeekdays = weekdays
	}
	if raw, ok := fields["custom_form_sections"]; ok {
		patch.settings.SetCustomFormSections = true
		if !isJSONNull(raw) {
			patch.settings.CustomFormSections = append([]byte(nil), raw...)
		}
	}
	if raw, ok := fields["theme"]; ok {
		var theme string
		if err := json.Unmarshal(raw, &theme); err != nil || !slices.Contains(validThemes, theme) {
			return practiceSettingsPatch{}, errors.New("Invalid theme")
		}
		patch.settings.SetTheme = true
		patch.settings.Theme = theme
		if colors, ok := fields["theme_colors"]; ok && !isJSONNull(colors) {
			validated, err := validateThemeColors(colors)
			if err != nil {
				return practiceSettingsPatch{}, err
			}
			patch.settings.ThemeColors = validated
		}
	}

	return patch, nil
}

func decodeBoolean(raw json.RawMessage, field string) (bool, error) {
	var value bool
	if isJSONNull(raw) {
		return false, fmt.Errorf("Invalid %s", field)
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("Invalid %s", field)
	}
	return value, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func normalizeAvailableWeekdays(raw json.RawMessage) ([]int16, error) {
	var input []int
	if err := json.Unmarshal(raw, &input); err != nil || input == nil {
		return nil, errors.New("Invalid available weekdays")
	}
	slices.Sort(input)
	input = slices.Compact(input)
	weekdays := make([]int16, len(input))
	for index, day := range input {
		if day < 0 || day > 6 {
			return nil, errors.New("Invalid available weekdays")
		}
		weekdays[index] = int16(day)
	}
	return weekdays, nil
}

func validateThemeColors(raw json.RawMessage) ([]byte, error) {
	var colors themeColorsInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&colors); err != nil || ensureJSONEnd(decoder) != nil {
		return nil, errors.New("Invalid theme colors")
	}
	for _, color := range []string{colors.Start, colors.Mid, colors.End, colors.Button, colors.Accent} {
		if !hexColorPattern.MatchString(color) {
			return nil, errors.New("Invalid theme colors")
		}
	}
	if !slices.Contains(validDirections, colors.Dir) {
		return nil, errors.New("Invalid theme colors")
	}
	validated, err := json.Marshal(colors)
	if err != nil {
		return nil, errors.New("Invalid theme colors")
	}
	return validated, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func respondPracticeSettingsError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}

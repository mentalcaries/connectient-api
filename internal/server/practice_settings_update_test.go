package server

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeAvailableWeekdays(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		want    []int16
		wantErr bool
	}{
		{name: "sort and deduplicate", value: `[6,1,3,1,0]`, want: []int16{0, 1, 3, 6}},
		{name: "empty is valid", value: `[]`, want: []int16{}},
		{name: "null", value: `null`, wantErr: true},
		{name: "not an array", value: `"1"`, wantErr: true},
		{name: "fraction", value: `[1.5]`, wantErr: true},
		{name: "out of range", value: `[7]`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeAvailableWeekdays(json.RawMessage(tc.value))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %t", err, tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("weekdays = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidateThemeColors(t *testing.T) {
	valid := `{"start":"#d1fae5","mid":"#eff6ff","end":"#e0f2fe","btn":"#7ab0d4","dir":"to bottom left","accent":"#dce6ed"}`
	for _, tc := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid", value: valid},
		{name: "invalid color", value: strings.Replace(valid, "#d1fae5", "red", 1), wantErr: true},
		{name: "invalid direction", value: strings.Replace(valid, "to bottom left", "sideways", 1), wantErr: true},
		{name: "missing field", value: `{"start":"#d1fae5"}`, wantErr: true},
		{name: "unknown field", value: strings.TrimSuffix(valid, "}") + `,"other":true}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateThemeColors(json.RawMessage(tc.value))
			if (err != nil) != tc.wantErr {
				t.Errorf("error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}

func TestDecodePracticeSettingsPatch(t *testing.T) {
	validColors := `{"start":"#d1fae5","mid":"#eff6ff","end":"#e0f2fe","btn":"#7ab0d4","dir":"to bottom left","accent":"#dce6ed"}`
	for _, tc := range []struct {
		name    string
		body    string
		wantErr string
		check   func(*testing.T, practiceSettingsPatch)
	}{
		{
			name: "all supported shapes",
			body: `{"specialty":null,"has_multiple_providers":false,"dental_history_enabled":true,"available_weekdays":[5,1,1],"custom_form_sections":null,"theme":"match_logo","theme_colors":` + validColors + `}`,
			check: func(t *testing.T, patch practiceSettingsPatch) {
				if !patch.hasPracticeUpdates() || !patch.hasSettingsUpdates() || patch.practice.Specialty != nil ||
					!patch.settings.DentalHistoryEnabled || !slices.Equal(patch.settings.AvailableWeekdays, []int16{1, 5}) ||
					patch.settings.CustomFormSections != nil || len(patch.settings.ThemeColors) == 0 {
					t.Errorf("unexpected patch: %+v", patch)
				}
			},
		},
		{name: "unknown fields only", body: `{"ignored":true}`},
		{name: "invalid boolean", body: `{"dental_history_enabled":"true"}`, wantErr: "Invalid dental_history_enabled"},
		{name: "null boolean", body: `{"dental_history_enabled":null}`, wantErr: "Invalid dental_history_enabled"},
		{name: "invalid specialty", body: `{"specialty":12}`, wantErr: "Invalid specialty"},
		{name: "invalid theme", body: `{"theme":"unknown"}`, wantErr: "Invalid theme"},
		{name: "colors ignored without theme", body: `{"theme_colors":` + validColors + `}`},
		{name: "trailing JSON", body: `{} {}`, wantErr: "Invalid request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			patch, err := decodePracticeSettingsPatch(strings.NewReader(tc.body))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if tc.check != nil {
				tc.check(t, patch)
			}
		})
	}
}

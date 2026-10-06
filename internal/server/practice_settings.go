package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type PracticeSettingsPractice struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	PracticeCategory string    `json:"practice_category"`
	Specialty        *string   `json:"specialty"`
	PracticeCode     string    `json:"practice_code"`
	City             string    `json:"city"`
}

type PracticeSettingsDetails struct {
	ID                          uuid.UUID       `json:"id"`
	CreatedAt                   time.Time       `json:"created_at"`
	UpdatedAt                   time.Time       `json:"updated_at"`
	PracticeID                  uuid.UUID       `json:"practice_id"`
	DentalHistoryEnabled        bool            `json:"dental_history_enabled"`
	TMJHistoryEnabled           bool            `json:"tmj_history_enabled"`
	MultipleLocationsEnabled    bool            `json:"multiple_locations_enabled"`
	OptometryHistoryEnabled     bool            `json:"optometry_history_enabled"`
	PhysiotherapyHistoryEnabled bool            `json:"physiotherapy_history_enabled"`
	CustomFormSections          json.RawMessage `json:"custom_form_sections"`
	Theme                       string          `json:"theme"`
	ThemeColors                 json.RawMessage `json:"theme_colors"`
	AvailableWeekdays           []int16         `json:"available_weekdays"`
}

type PracticeSettingsLocation struct {
	ID                uuid.UUID  `json:"id"`
	CreatedAt         time.Time  `json:"created_at"`
	DeletedAt         *time.Time `json:"deleted_at"`
	PracticeID        uuid.UUID  `json:"practice_id"`
	Name              string     `json:"name"`
	Address           string     `json:"address"`
	IsActive          bool       `json:"is_active"`
	SortOrder         int32      `json:"sort_order"`
	AvailableWeekdays []int16    `json:"available_weekdays"`
}

type PracticeSettingsData struct {
	Practice       PracticeSettingsPractice   `json:"practice"`
	Settings       PracticeSettingsDetails    `json:"settings"`
	ProcedureTypes []ProcedureType            `json:"procedure_types"`
	Locations      []PracticeSettingsLocation `json:"locations"`
	IsAdmin        bool                       `json:"is_admin"`
}

func (s *Server) handlerGetPracticeSettings(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}

	practice, err := s.DBQuery.GetPracticeSettingsOverview(c, *user.PracticeId)
	if errors.Is(err, pgx.ErrNoRows) {
		respondWithError(c, http.StatusNotFound, "Practice not found", nil)
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch practice", err)
		return
	}

	settings, err := s.DBQuery.GetPracticeSettings(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch practice settings", err)
		return
	}
	procedures, err := s.DBQuery.GetProcedureTypesByPracticeID(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch procedure types", err)
		return
	}
	locations, err := s.DBQuery.GetPracticeLocationsByPracticeID(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch locations", err)
		return
	}

	role := ""
	if user.Role != nil {
		role = *user.Role
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": PracticeSettingsData{
			Practice: PracticeSettingsPractice{
				ID: practice.ID, Name: practice.Name, PracticeCategory: practice.PracticeCategory,
				Specialty: practice.Specialty, PracticeCode: practice.PracticeCode, City: practice.City,
			},
			Settings:       practiceSettingsDetails(settings),
			ProcedureTypes: procedureTypeResponses(procedures),
			Locations:      practiceLocationResponses(locations),
			IsAdmin:        role == "owner" || role == "admin",
		},
	})
}

func practiceSettingsDetails(settings db.PracticeSetting) PracticeSettingsDetails {
	return PracticeSettingsDetails{
		ID: settings.ID, CreatedAt: settings.CreatedAt, UpdatedAt: settings.UpdatedAt,
		PracticeID: settings.PracticeID, DentalHistoryEnabled: settings.DentalHistoryEnabled,
		TMJHistoryEnabled:           settings.TmjHistoryEnabled,
		MultipleLocationsEnabled:    settings.MultipleLocationsEnabled,
		OptometryHistoryEnabled:     settings.OptometryHistoryEnabled,
		PhysiotherapyHistoryEnabled: settings.PhysiotherapyHistoryEnabled,
		CustomFormSections:          json.RawMessage(settings.CustomFormSections), Theme: settings.Theme,
		ThemeColors: json.RawMessage(settings.ThemeColors), AvailableWeekdays: settings.AvailableWeekdays,
	}
}

func procedureTypeResponses(rows []db.ProcedureType) []ProcedureType {
	result := make([]ProcedureType, 0, len(rows))
	for _, row := range rows {
		result = append(result, ProcedureType{
			ID: row.ID, CreatedAt: row.CreatedAt, DeletedAt: row.DeletedAt,
			PracticeID: row.PracticeID, Name: row.Name, Value: row.Value,
			IsActive: row.IsActive, IsDefault: row.IsDefault, IsPrimary: row.IsPrimary,
			SortOrder: int(row.SortOrder),
		})
	}
	return result
}

func practiceLocationResponses(rows []db.PracticeLocation) []PracticeSettingsLocation {
	result := make([]PracticeSettingsLocation, 0, len(rows))
	for _, row := range rows {
		result = append(result, PracticeSettingsLocation{
			ID: row.ID, CreatedAt: row.CreatedAt, DeletedAt: row.DeletedAt,
			PracticeID: row.PracticeID, Name: row.Name, Address: row.Address,
			IsActive: row.IsActive, SortOrder: row.SortOrder,
			AvailableWeekdays: row.AvailableWeekdays,
		})
	}
	return result
}

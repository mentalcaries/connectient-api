package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Practice struct {
	ID                   uuid.UUID `json:"id"`
	CreatedAt            time.Time `json:"created_at"`
	ModifiedAt           time.Time `json:"modified_at"`
	Name                 string    `json:"name"`
	City                 string    `json:"city"`
	Phone                *string   `json:"phone"`
	Email                *string   `json:"email"`
	PracticeCode         string    `json:"practice_code"`
	Logo                 *string   `json:"logo"`
	StreetAddress        *string   `json:"street_address"`
	Facebook             *string   `json:"facebook"`
	Instagram            *string   `json:"instagram"`
	Website              *string   `json:"website"`
	HasMultipleProviders bool      `json:"has_multiple_providers"`
	Specialty            *string   `json:"specialty"`
	IsSuspended          bool      `json:"is_suspended"`
	PracticeCategory     string    `json:"practice_category"`
	IsActive             bool      `json:"is_active"`
}

type PracticeWithSettings struct {
	ID                          uuid.UUID        `json:"id"`
	CreatedAt                   time.Time        `json:"created_at"`
	ModifiedAt                  time.Time        `json:"modified_at"`
	Name                        string           `json:"name"`
	City                        string           `json:"city"`
	Phone                       *string          `json:"phone"`
	Email                       *string          `json:"email"`
	PracticeCode                string           `json:"practice_code"`
	Logo                        *string          `json:"logo"`
	StreetAddress               *string          `json:"street_address"`
	Facebook                    *string          `json:"facebook"`
	Instagram                   *string          `json:"instagram"`
	Website                     *string          `json:"website"`
	HasMultipleProviders        bool             `json:"has_multiple_providers"`
	Specialty                   *string          `json:"specialty"`
	IsSuspended                 bool             `json:"is_suspended"`
	PracticeCategory            string           `json:"practice_category"`
	IsActive                    bool             `json:"is_active"`
	DentalHistoryEnabled        bool             `json:"dental_history_enabled"`
	TMJHistoryEnabled           bool             `json:"tmj_history_enabled"`
	MultipleLocationsEnabled    bool             `json:"multiple_locations_enabled"`
	OptometryHistoryEnabled     bool             `json:"optometry_history_enabled"`
	PhysiotherapyHistoryEnabled bool             `json:"physiotherapy_history_enabled"`
	CustomFormSections          *json.RawMessage `json:"custom_form_sections"`
	Theme                       string           `json:"theme"`
	ThemeColors                 *json.RawMessage `json:"theme_colors"`
}

type UpdatePracticeParams struct {
	Name                 *string `json:"name,omitempty"`
	StreetAddress        *string `json:"street_address,omitempty"`
	City                 *string `json:"city,omitempty"`
	Phone                *string `json:"phone,omitempty"`
	Email                *string `json:"email,omitempty"`
	Website              *string `json:"website,omitempty"`
	Facebook             *string `json:"facebook,omitempty"`
	Instagram            *string `json:"instagram,omitempty"`
	Specialty            *string `json:"specialty,omitempty"`
	PracticeCode         *string `json:"practice_code,omitempty"`
	HasMultipleProviders *bool   `json:"has_multiple_providers,omitempty"`
}

func (s *Server) handlerGetPracticeWithSettings(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)

	practice, err := s.DBQuery.GetPractice(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "cannot get practice data", err)
		return
	}

	c.JSON(http.StatusOK, practice)

}

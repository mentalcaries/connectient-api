package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type PublicPractice struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	Logo                 *string   `json:"logo"`
	City                 string    `json:"city"`
	StreetAddress        *string   `json:"street_address"`
	Phone                *string   `json:"phone"`
	Email                *string   `json:"email"`
	Website              *string   `json:"website"`
	PracticeCode         string    `json:"practice_code"`
	Instagram            *string   `json:"instagram"`
	Facebook             *string   `json:"facebook"`
	HasMultipleProviders bool      `json:"has_multiple_providers"`
	PracticeCategory     string    `json:"practice_category"`
	Specialty            *string   `json:"specialty"`
}

type PublicBookingSettings struct {
	MultipleLocationsEnabled bool            `json:"multiple_locations_enabled"`
	AvailableWeekdays        []int16         `json:"available_weekdays"`
	Theme                    string          `json:"theme"`
	ThemeColors              json.RawMessage `json:"theme_colors"`
}

type PublicProvider struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Title     *string   `json:"title"`
	Specialty string    `json:"specialty"`
}

type PublicProcedureType struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Value     string    `json:"value"`
	SortOrder int32     `json:"sort_order"`
	IsPrimary bool      `json:"is_primary"`
}

type PublicLocation struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	SortOrder         int32     `json:"sort_order"`
	AvailableWeekdays []int16   `json:"available_weekdays"`
}

func (s *Server) handlerGetPublicBookingConfig(c *gin.Context) {
	practice, ok := s.publicBookingPractice(c)
	if !ok {
		return
	}
	procedures, err := s.DBQuery.GetPublicProcedureTypes(c, practice.ID)
	if err != nil {
		respondPublicBookingError(c, http.StatusInternalServerError, "Failed to fetch booking configuration", err)
		return
	}
	providers := []PublicProvider{}
	if practice.HasMultipleProviders {
		rows, err := s.DBQuery.GetPublicProviders(c, practice.ID)
		if err != nil {
			respondPublicBookingError(c, http.StatusInternalServerError, "Failed to fetch booking configuration", err)
			return
		}
		providers = publicProviderResponses(rows)
	}
	locations := []PublicLocation{}
	if practice.MultipleLocationsEnabled {
		rows, err := s.DBQuery.GetPublicLocations(c, practice.ID)
		if err != nil {
			respondPublicBookingError(c, http.StatusInternalServerError, "Failed to fetch booking configuration", err)
			return
		}
		locations = publicLocationResponses(rows)
	}
	c.JSON(http.StatusOK, gin.H{
		"practice": publicPracticeResponse(practice),
		"settings": PublicBookingSettings{
			MultipleLocationsEnabled: practice.MultipleLocationsEnabled,
			AvailableWeekdays:        practice.AvailableWeekdays,
			Theme:                    practice.Theme, ThemeColors: json.RawMessage(practice.ThemeColors),
		},
		"providers":       providers,
		"procedure_types": publicProcedureResponses(procedures),
		"locations":       locations,
	})
}

func (s *Server) handlerGetPublicProcedureTypes(c *gin.Context) {
	practice, ok := s.publicBookingPractice(c)
	if !ok {
		return
	}
	rows, err := s.DBQuery.GetPublicProcedureTypes(c, practice.ID)
	if err != nil {
		respondPublicBookingError(c, http.StatusInternalServerError, "Failed to fetch procedure types", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": publicProcedureResponses(rows)})
}

func (s *Server) handlerGetPublicLocations(c *gin.Context) {
	practice, ok := s.publicBookingPractice(c)
	if !ok {
		return
	}
	locations := []PublicLocation{}
	if practice.MultipleLocationsEnabled {
		rows, err := s.DBQuery.GetPublicLocations(c, practice.ID)
		if err != nil {
			respondPublicBookingError(c, http.StatusInternalServerError, "Failed to fetch locations", err)
			return
		}
		locations = publicLocationResponses(rows)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "data": locations,
		"multiple_locations_enabled": practice.MultipleLocationsEnabled,
		"available_weekdays":         practice.AvailableWeekdays,
	})
}

func (s *Server) publicBookingPractice(c *gin.Context) (db.GetPublicBookingPracticeRow, bool) {
	practice, err := s.DBQuery.GetPublicBookingPractice(c, c.Param("code"))
	if errors.Is(err, pgx.ErrNoRows) {
		respondPublicBookingError(c, http.StatusNotFound, "Practice not found", nil)
		return db.GetPublicBookingPracticeRow{}, false
	}
	if err != nil {
		respondPublicBookingError(c, http.StatusInternalServerError, "Failed to fetch practice", err)
		return db.GetPublicBookingPracticeRow{}, false
	}
	subscription := computeSubscriptionContext(
		practice.SubscriptionStatus, practice.SubscriptionPlan,
		nullableTimestamp(practice.SubscriptionTrialEnd),
		nullableTimestamp(practice.SubscriptionPeriodEnd),
		nullableTimestamp(practice.SubscriptionCancelAt), time.Now(),
	)
	if !practice.IsActive || practice.IsSuspended || !subscription.CanAccessBookings {
		respondPublicBookingError(c, http.StatusNotFound, "Practice not found", nil)
		return db.GetPublicBookingPracticeRow{}, false
	}
	return practice, true
}

func publicPracticeResponse(row db.GetPublicBookingPracticeRow) PublicPractice {
	return PublicPractice{
		ID: row.ID, Name: row.Name, Logo: row.Logo, City: row.City,
		StreetAddress: row.StreetAddress, Phone: row.Phone, Email: row.Email,
		Website: row.Website, PracticeCode: row.PracticeCode,
		Instagram: row.Instagram, Facebook: row.Facebook,
		HasMultipleProviders: row.HasMultipleProviders,
		PracticeCategory:     row.PracticeCategory, Specialty: row.Specialty,
	}
}

func publicProviderResponses(rows []db.GetPublicProvidersRow) []PublicProvider {
	result := make([]PublicProvider, 0, len(rows))
	for _, row := range rows {
		result = append(result, PublicProvider{
			ID: row.ID, FirstName: row.FirstName, LastName: row.LastName,
			Title: row.Title, Specialty: row.Specialty,
		})
	}
	return result
}

func publicProcedureResponses(rows []db.GetPublicProcedureTypesRow) []PublicProcedureType {
	result := make([]PublicProcedureType, 0, len(rows))
	for _, row := range rows {
		result = append(result, PublicProcedureType{
			ID: row.ID, Name: row.Name, Value: row.Value,
			SortOrder: row.SortOrder, IsPrimary: row.IsPrimary,
		})
	}
	return result
}

func publicLocationResponses(rows []db.GetPublicLocationsRow) []PublicLocation {
	result := make([]PublicLocation, 0, len(rows))
	for _, row := range rows {
		result = append(result, PublicLocation{
			ID: row.ID, Name: row.Name, SortOrder: row.SortOrder,
			AvailableWeekdays: row.AvailableWeekdays,
		})
	}
	return result
}

func respondPublicBookingError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}

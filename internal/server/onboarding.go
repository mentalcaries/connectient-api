package server

import (
	"errors"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/mentalcaries/connectient-api/internal/database"
)

type OnboardingCompletionRequest struct {
	IsSoloProvider   bool    `json:"is_solo_provider"`
	Name             string  `json:"name"`
	PracticeCategory string  `json:"practice_category"`
	Specialty        *string `json:"specialty"`
	PracticeCode     string  `json:"practice_code"`
	City             string  `json:"city"`
	FirstName        string  `json:"first_name"`
	LastName         string  `json:"last_name"`
	MobilePhone      string  `json:"mobile_phone"`
	TermsAgreed      bool    `json:"termsAgreed"`
}

func (s *Server) handlerCompleteOnboarding(c *gin.Context) {
	setPrivateNoStore(c)
	claims := c.MustGet("claims").(TokenClaims)

	var req OnboardingCompletionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		respondOnboardingError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	if req.Name == "" || req.PracticeCategory == "" || req.PracticeCode == "" || req.City == "" ||
		req.FirstName == "" || req.LastName == "" || req.MobilePhone == "" {
		respondOnboardingError(c, http.StatusBadRequest, "Missing required fields", nil)
		return
	}
	if !req.TermsAgreed {
		respondOnboardingError(c, http.StatusBadRequest, "terms_required", nil)
		return
	}
	if claims.Email == "" {
		respondOnboardingError(c, http.StatusUnauthorized, "Authentication required", nil)
		return
	}
	if _, ok := DefaultSettingsByCategory[req.PracticeCategory]; !ok {
		respondOnboardingError(c, http.StatusBadRequest, "Invalid practice category", nil)
		return
	}

	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to complete onboarding", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	currentTime := time.Now()
	role := "owner"

	user, err := queries.CreateUser(c, db.CreateUserParams{
		ID:            claims.ID,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Email:         &claims.Email,
		MobilePhone:   &req.MobilePhone,
		TermsAgreedAt: &currentTime,
		Role:          &role,
	})

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_pkey" {
			respondOnboardingError(c, http.StatusConflict, "Onboarding already completed for this account", err)
			return
		}
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to create user record", err)
		return
	}

	if slices.Contains(RESERVED_CODES, req.PracticeCode) {
		respondOnboardingError(c, http.StatusConflict, "Practice code is already taken", nil)
		return
	}

	createdPractice, err := queries.CreatePractice(c, db.CreatePracticeParams{
		Name:                 req.Name,
		City:                 req.City,
		PracticeCategory:     req.PracticeCategory,
		Specialty:            req.Specialty,
		PracticeCode:         req.PracticeCode,
		Email:                &claims.Email,
		HasMultipleProviders: !req.IsSoloProvider,
	})

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondOnboardingError(c, http.StatusConflict, "Practice code is already taken", err)
			return
		}
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to create practice", err)
		return
	}

	_, err = queries.UpdateUserPracticeID(c, db.UpdateUserPracticeIDParams{
		ID:         user.ID,
		PracticeID: &createdPractice.ID,
	})

	if err != nil {
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to update user record", err)
		return
	}

	defaultSettings := DefaultSettingsByCategory[createdPractice.PracticeCategory]
	defaultProcedures := DefaultProcedureTypes[createdPractice.PracticeCategory]

	_, err = queries.CreatePracticeSettings(c, db.CreatePracticeSettingsParams{
		PracticeID:                  createdPractice.ID,
		DentalHistoryEnabled:        defaultSettings.DentalHistoryEnabled,
		TmjHistoryEnabled:           defaultSettings.TMJHistoryEnabled,
		MultipleLocationsEnabled:    defaultSettings.MultipleLocationsEnabled,
		PhysiotherapyHistoryEnabled: defaultSettings.PhysiotherapyHistoryEnabled,
		OptometryHistoryEnabled:     defaultSettings.OptometryHistoryEnabled,
		CustomFormSections:          defaultSettings.CustomFormSections,
	})

	if err != nil {
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to create practice settings", err)
		return
	}

	for _, procedure := range defaultProcedures {
		_, err := queries.CreateProcedureType(c, db.CreateProcedureTypeParams{
			PracticeID: createdPractice.ID,
			Name:       procedure.Name,
			Value:      procedure.Value,
			SortOrder:  int32(procedure.SortOrder),
			IsPrimary:  procedure.IsPrimary,
			IsActive:   true,
			IsDefault:  true,
		})
		if err != nil {
			respondOnboardingError(c, http.StatusInternalServerError, "Failed to create procedure types", err)
			return
		}
	}

	const trialDurationDays = 30
	trialStart := time.Now()

	err = queries.CreateSubscription(c, db.CreateSubscriptionParams{
		ReferenceID: createdPractice.ID.String(),
		Plan:        "pro",
		Status:      "trialing",
		TrialStart:  pgtype.Timestamptz{Time: trialStart, Valid: true},
		TrialEnd:    pgtype.Timestamptz{Time: trialStart.AddDate(0, 0, trialDurationDays), Valid: true},
	})
	if err != nil {
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to create subscription", err)
		return
	}

	if err := tx.Commit(c); err != nil {
		respondOnboardingError(c, http.StatusInternalServerError, "Failed to complete onboarding", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true, "practice_id": createdPractice.ID, "redirect_to": "/admin/dashboard",
	})
}

func respondOnboardingError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}

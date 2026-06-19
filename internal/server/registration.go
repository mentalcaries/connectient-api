package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/mentalcaries/connectient-api/internal/database"
)

type RegisterRequest struct {
	IsSoloProvider   bool    `json:"is_solo_provider"`
	Name             string  `json:"name" binding:"required"`
	PracticeCategory string  `json:"practice_category" binding:"required"`
	Specialty        *string `json:"specialty"`
	PracticeCode     string  `json:"practice_code" binding:"required"`
	City             string  `json:"city" binding:"required"`
	FirstName        string  `json:"first_name" binding:"required"`
	LastName         string  `json:"last_name" binding:"required"`
	MobilePhone      string  `json:"mobile_phone" binding:"required"`
	TermsAgreed      bool    `json:"terms_agreed" binding:"required"`
}

type CreateUserParams struct {
	ID            uuid.UUID
	Email         string     `json:"email" binding:"required"`
	FirstName     string     `json:"first_name" binding:"required"`
	LastName      string     `json:"last_name" binding:"required"`
	MobilePhone   string     `json:"mobile_phone" binding:"required"`
	PracticeID    *uuid.UUID `json:"practice_id"`
	Role          string     `json:"role"`
	TermsAgreedAt time.Time  `json:"terms_agreed_at" binding:"required"`
}

type CreatePracticeParams struct {
	Name                 string  `json:"name"`
	Email                string  `json:"email"`
	PracticeCategory     string  `json:"practice_category"`
	Specialty            *string `json:"specialty"`
	PracticeCode         string  `json:"practice_code"`
	City                 string  `json:"city"`
	HasMultipleProviders bool    `json:"has_multiple_providers"`
}

type CreatePracticeSettingsParams struct {
	PracticeID                  uuid.UUID        `json:"practice_id"`
	DentalHistoryEnabled        bool             `json:"dental_history_enabled"`
	TMJHistoryEnabled           bool             `json:"tmj_history_enabled"`
	MultipleLocationsEnabled    bool             `json:"multiple_locations_enabled"`
	OptometryHistoryEnabled     bool             `json:"optometry_history_enabled"`
	PhysiotherapyHistoryEnabled bool             `json:"physiotherapy_history_enabled"`
	CustomFormSections          *json.RawMessage `json:"custom_form_sections"`
	Theme                       string           `json:"theme"`
	ThemeColors                 *json.RawMessage `json:"theme_colors"`
}

type CreateProcedureTypeParams struct {
	PracticeID uuid.UUID `json:"practice_id"`
	Name       string    `json:"name"`
	Value      string    `json:"value"`
	IsPrimary  bool      `json:"is_primary"`
}

type CreateSubscriptionParams struct {
	ReferenceID uuid.UUID `json:"reference_id"`
	Plan        string    `json:"plan"`
	Status      string    `json:"status"`
	TrialStart  time.Time `json:"trial_start"`
	TrialEnd    time.Time `json:"trial_end"`
}

func (s *Server) handlerNewRegistration(c *gin.Context) {
	claims := c.MustGet("claims").(TokenClaims)

	var req RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "invalid request", err)
		return
	}

	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not begin transaction", err)
		return
	}
	defer tx.Rollback(c)


	currentTime := time.Now()
	role := "owner"

	user, err := s.DBQuery.CreateUser(c, db.CreateUserParams{
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
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondWithError(c, http.StatusConflict, "user already exists", err)
			return
		}
		respondWithError(c, http.StatusInternalServerError, "could not create user", err)
		return
	}

	if slices.Contains(RESERVED_CODES, req.PracticeCode) {
		respondWithError(c, http.StatusConflict, "practice code already in use", err)
		return
	}

	createdPractice, err := s.DBQuery.CreatePractice(c, db.CreatePracticeParams{
		Name:                 req.Name,
		City:                 req.City,
		PracticeCategory:     req.PracticeCategory,
		Specialty:            req.Specialty,
		PracticeCode:         req.PracticeCode,
		HasMultipleProviders: !req.IsSoloProvider,
	})

	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not create practice", err)
		return
	}

	_, err = s.DBQuery.UpdateUserPracticeID(c, db.UpdateUserPracticeIDParams{
		ID:         user.ID,
		PracticeID: &createdPractice.ID,
	})

	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not update user's practice id", err)
		return
	}

	defaultSettings := DefaultSettingsByCategory[createdPractice.PracticeCategory]
	defaultProcedures := DefaultProcedureTypes[createdPractice.PracticeCategory]

	_, err = s.DBQuery.CreatePracticeSettings(c, db.CreatePracticeSettingsParams{
		PracticeID:                  createdPractice.ID,
		DentalHistoryEnabled:        defaultSettings.DentalHistoryEnabled,
		TmjHistoryEnabled:           defaultSettings.TMJHistoryEnabled,
		MultipleLocationsEnabled:    defaultSettings.MultipleLocationsEnabled,
		PhysiotherapyHistoryEnabled: defaultSettings.PhysiotherapyHistoryEnabled,
		OptometryHistoryEnabled:     defaultSettings.OptometryHistoryEnabled,
		CustomFormSections:          defaultSettings.CustomFormSections,
	})

	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not create practice settings", err)
		return
	}

	for _, procedure := range defaultProcedures {
		_, err := s.DBQuery.CreateProcedureType(c, db.CreateProcedureTypeParams{
			PracticeID: createdPractice.ID,
			Name:       procedure.Name,
			Value:      procedure.Value,
			SortOrder:  int32(procedure.SortOrder),
			IsPrimary:  procedure.IsPrimary,
			IsActive:   true,
			IsDefault:  true,
		})
		if err != nil {
			respondWithError(c, http.StatusInternalServerError, "could not create practice procedure", err)
			return
		}
	}

	const trialDurationDays = 30

	err = s.DBQuery.CreateSubscription(c, db.CreateSubscriptionParams{
		ReferenceID: createdPractice.ID.String(),
		Plan:        "pro",
		Status:      "trialing",
		TrialStart:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
		TrialEnd:    pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, trialDurationDays), Valid: true},
	})
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not create subscription", err)
		return
	}

	if err := tx.Commit(c); err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not commit transaction", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Successfully created " + createdPractice.Name, "practiceId": createdPractice.ID})
}

// # 1. Validate required fields
// if req.Name == "" or req.PracticeCategory == "" or req.PracticeCode == ""
//    or req.City == "" or req.FirstName == "" or req.LastName == "" or req.MobilePhone == "":
//     return 400 "missing required fields"

// if not req.TermsAgreed:
//     return 400 "terms_required"

// # 2. Check user hasn't already onboarded
// if userID already exists in Users:
//     return 409 "onboarding already completed"

// # 3. Reject reserved practice codes (app routes — cannot collide)
// if req.PracticeCode in RESERVED_CODES:
//     return 409 "practice code unavailable"

// # 4. Begin transaction
// tx := db.Begin()
// defer tx.Rollback() on error

// user := CreateUser(tx, ...)               # practice_id = null, role = owner
// practice := CreatePractice(tx, ...)        # UNIQUE constraint catches duplicate code
// UpdateUserPractice(tx, user.ID, practice.ID)
// CreatePracticeSettings(tx, defaultsFor(req.PracticeCategory))
// CreateProcedureTypesBulk(tx, defaultsFor(req.PracticeCategory))
// CreateSubscription(tx, practice.ID, plan: "pro", status: "trialing", trial: 30 days)

// tx.Commit()

// return 201 { practice_id: practice.ID, redirect_to: "/admin/dashboard" }

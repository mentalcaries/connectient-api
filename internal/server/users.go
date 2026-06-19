package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type User struct {
	ID                           uuid.UUID  `json:"id"`
	CreatedAt                    *time.Time `json:"created_at,omitempty"`
	FirstName                    string     `json:"first_name"`
	LastName                     string     `json:"last_name"`
	MobilePhone                  *string    `json:"mobile_phone,omitempty"`
	Email                        *string    `json:"email,omitempty"`
	PracticeID                   *uuid.UUID `json:"practice_id,omitempty"`
	Role                         *string    `json:"role,omitempty"`
	OrgRole                      *string    `json:"org_role,omitempty"`
	IsActive                     bool       `json:"is_active"`
	InvitedBy                    *uuid.UUID `json:"invited_by,omitempty"`
	DeletedAt                    *time.Time `json:"deleted_at,omitempty"`
	AvatarURL                    *string    `json:"avatar_url,omitempty"`
	WhatsappNotificationsEnabled bool       `json:"whatsapp_notifications_enabled"`
	TermsAgreedAt                *time.Time `json:"terms_agreed_at,omitempty"`
}

type CurrentUser struct {
	ID          uuid.UUID  `json:"id"`
	Email       *string    `json:"email"`
	Name        string     `json:"name"`
	PracticeId  *uuid.UUID `json:"practice_id"`
	Role        *string    `json:"role"`
	IsActive    *bool      `json:"is_active"`
	DeletedAt   *time.Time `json:"deleted_at"`
	IsSuspended *bool      `json:"is_suspended"`
	Status      *string    `json:"status"`
	TrialEnd    *time.Time `json:"trial_end"`
	PeriodEnd   *time.Time `json:"period_end"`
}

type CreateInvitedUserParams struct {
	ID            uuid.UUID `json:"id"`
	PracticeID    uuid.UUID `json:"practice_id" binding:"required"`
	Email         string    `json:"email" binding:"required"`
	FirstName     string    `json:"first_name" binding:"required"`
	LastName      string    `json:"last_name" binding:"required"`
	MobilePhone   *string   `json:"mobile_phone,omitempty"`
	Role          string    `json:"role" binding:"required"`
	OrgRole       *string   `json:"org_role,omitempty"`
	InvitedBy     uuid.UUID `json:"invited_by" binding:"required"`
	IsActive      bool      `json:"is_active"`
	TermsAgreedAt time.Time `json:"terms_agreed_at"`
}

type UpdateUserPracticeParams struct {
	PracticeID uuid.UUID `json:"practice_id"`
	ID         uuid.UUID `json:"id"`
}

type UpdateUserRoleStatusParams struct {
	ID         uuid.UUID `json:"id"`
	PracticeID uuid.UUID `json:"practice_id"`
	Role       *string   `json:"role,omitempty"`
	IsActive   *bool     `json:"is_active,omitempty"`
}

type UpdateUserProfileParams struct {
	ID                           uuid.UUID `json:"id"`
	FirstName                    string    `json:"first_name"`
	LastName                     string    `json:"last_name"`
	AvatarURL                    *string   `json:"avatar_url"`
	MobilePhone                  *string   `json:"mobile_phone,omitempty"`
	WhatsappNotificationsEnabled *bool     `json:"whatsapp_notifications_enabled,omitempty"`
}

// func (s *Server) handlerUserCreate(c *gin.Context) {
// 	var userParams CreateUserParams

// 	if err := c.ShouldBindJSON(&userParams); err != nil {
// 		respondWithError(c, http.StatusBadRequest, "invalid request", err)
// 		return
// 	}

// 	currentTime := time.Now()

// 	user, err := s.DBQuery.CreateUser(c, db.CreateUserParams{
// 		ID:            userParams.ID,
// 		FirstName:     userParams.FirstName,
// 		LastName:      userParams.LastName,
// 		Email:         &userParams.Email,
// 		MobilePhone:   &userParams.MobilePhone,
// 		TermsAgreedAt: &currentTime,
// 		Role:          &userParams.Role,
// 	})

// 	if err != nil {
// 		respondWithError(c, http.StatusInternalServerError, "could not create user", err)
// 	}
// 	c.JSON(http.StatusCreated, User{
// 		ID:            user.ID,
// 		FirstName:     user.FirstName,
// 		LastName:      user.LastName,
// 		Email:         user.Email,
// 		MobilePhone:   user.MobilePhone,
// 		TermsAgreedAt: user.TermsAgreedAt,
// 		Role:          user.Role,
// 		CreatedAt:     user.CreatedAt,
// 	})
// }

func (s *Server) handlerGetCurrentUser(c *gin.Context) {
	claims := c.MustGet("claims").(TokenClaims)

	user, err := s.DBQuery.GetUserWithSubscriptionStatus(c, claims.ID)
	if err != nil {
		respondWithError(c, http.StatusNotFound, "user not found", err)
		return
	}

	c.JSON(http.StatusOK, CurrentUser{
		ID:          user.ID,
		Email:       user.Email,
		Name:        user.FirstName + " " + user.LastName,
		PracticeId:  user.PracticeID,
		Role:        user.Role,
		IsSuspended: user.PracticeIsSuspended,
		IsActive:    &user.IsActive,
		DeletedAt:   user.DeletedAt,
		Status:      user.SubscriptionStatus,
		TrialEnd:    &user.SubscriptionTrialEnd.Time,
		PeriodEnd:   &user.SubscriptionPeriodEnd.Time,
	})
}

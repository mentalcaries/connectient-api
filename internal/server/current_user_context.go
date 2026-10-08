package server

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const subscriptionGracePeriod = 30 * 24 * time.Hour

type IdentityContext struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type MembershipContext struct {
	ID            uuid.UUID  `json:"id"`
	PracticeID    *uuid.UUID `json:"practice_id"`
	Role          *string    `json:"role"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	AvatarURL     *string    `json:"avatar_url"`
	IsActive      bool       `json:"is_active"`
	DeletedAt     *time.Time `json:"deleted_at"`
	AccessRevoked bool       `json:"access_revoked"`
}

type PracticeContext struct {
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
	IsSuspended          bool      `json:"is_suspended"`
}

type SubscriptionContext struct {
	Status                 string     `json:"status"`
	Plan                   *string    `json:"plan"`
	TrialEndsAt            *time.Time `json:"trialEndsAt"`
	DaysRemaining          *int       `json:"daysRemaining"`
	IsActive               bool       `json:"isActive"`
	IsExpired              bool       `json:"isExpired"`
	IsInGracePeriod        bool       `json:"isInGracePeriod"`
	GracePeriodEndsAt      *time.Time `json:"gracePeriodEndsAt"`
	ExpiryDate             *time.Time `json:"expiryDate"`
	WasTrialing            bool       `json:"wasTrialing"`
	CanAccessBookings      bool       `json:"canAccessBookings"`
	CanAccessRegistrations bool       `json:"canAccessRegistrations"`
	CanAccessCalendar      bool       `json:"canAccessCalendar"`
	ShowTrialBanner        bool       `json:"showTrialBanner"`
	ShowExpiredBanner      bool       `json:"showExpiredBanner"`
	CancelAt               *time.Time `json:"cancelAt"`
}

type CurrentUserContext struct {
	Identity           IdentityContext      `json:"identity"`
	Membership         *MembershipContext   `json:"membership"`
	Practice           *PracticeContext     `json:"practice"`
	Subscription       *SubscriptionContext `json:"subscription"`
	Permissions        []string             `json:"permissions"`
	OnboardingRequired bool                 `json:"onboarding_required"`
}

func (s *Server) handlerGetCurrentUserContext(c *gin.Context) {
	claims := c.MustGet("claims").(TokenClaims)
	response := CurrentUserContext{
		Identity:           IdentityContext{ID: claims.ID, Email: claims.Email},
		Permissions:        []string{},
		OnboardingRequired: true,
	}

	row, err := s.DBQuery.GetCurrentUserContext(c, claims.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusOK, response)
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not load user context", err)
		return
	}

	practice := practiceContextFromRow(row)
	accessRevoked := membershipContextRevoked(row, practice)
	response.OnboardingRequired = false
	response.Membership = &MembershipContext{
		ID: row.ID, PracticeID: row.PracticeID, Role: row.Role,
		FirstName: row.FirstName, LastName: row.LastName, AvatarURL: row.AvatarUrl,
		IsActive: row.IsActive, DeletedAt: row.DeletedAt, AccessRevoked: accessRevoked,
	}
	response.Practice = practice
	if practice != nil {
		response.Subscription = computeSubscriptionContext(
			row.SubscriptionStatus, row.SubscriptionPlan,
			nullableTimestamp(row.SubscriptionTrialEnd),
			nullableTimestamp(row.SubscriptionPeriodEnd),
			nullableTimestamp(row.SubscriptionCancelAt),
			time.Now(),
		)
	}
	response.Permissions = contextPermissions(row.Role, !accessRevoked, response.Subscription)
	c.JSON(http.StatusOK, response)
}

func practiceContextFromRow(row db.GetCurrentUserContextRow) *PracticeContext {
	if row.ContextPracticeID == nil || row.PracticeName == nil || row.PracticeCity == nil ||
		row.PracticeCode == nil || row.PracticeCategory == nil || row.HasMultipleProviders == nil {
		return nil
	}
	return &PracticeContext{
		ID: *row.ContextPracticeID, Name: *row.PracticeName, Logo: row.PracticeLogo,
		City: *row.PracticeCity, StreetAddress: row.PracticeStreetAddress,
		Phone: row.PracticePhone, Email: row.PracticeEmail, Website: row.PracticeWebsite,
		PracticeCode: *row.PracticeCode, Instagram: row.PracticeInstagram,
		Facebook: row.PracticeFacebook, HasMultipleProviders: *row.HasMultipleProviders,
		PracticeCategory: *row.PracticeCategory, Specialty: row.PracticeSpecialty,
		IsSuspended: row.IsSuspended != nil && *row.IsSuspended,
	}
}

func membershipContextRevoked(row db.GetCurrentUserContextRow, practice *PracticeContext) bool {
	if !row.IsActive || row.DeletedAt != nil || row.PracticeID == nil || row.Role == nil || practice == nil || practice.IsSuspended {
		return true
	}
	return *row.Role != "owner" && *row.Role != "admin" && *row.Role != "staff"
}

func computeSubscriptionContext(status, plan *string, trialEnd, periodEnd, cancelAt *time.Time, now time.Time) *SubscriptionContext {
	rawStatus := "none"
	if status != nil {
		rawStatus = *status
	}
	effectiveStatus := rawStatus
	if rawStatus == "trialing" && trialEnd != nil && trialEnd.Before(now) {
		effectiveStatus = "expired"
	}
	if rawStatus == "active" && periodEnd != nil && periodEnd.Before(now) {
		effectiveStatus = "expired"
	}
	isActive := effectiveStatus == "trialing" || effectiveStatus == "active"
	isExpired := effectiveStatus == "expired" || effectiveStatus == "past_due" || effectiveStatus == "canceled"
	var expiryDate *time.Time
	if rawStatus == "trialing" {
		expiryDate = trialEnd
	} else {
		expiryDate = periodEnd
	}
	var graceEnd *time.Time
	if expiryDate != nil {
		value := expiryDate.Add(subscriptionGracePeriod)
		graceEnd = &value
	}
	inGrace := isExpired && graceEnd != nil && now.Before(*graceEnd)
	var daysRemaining *int
	if effectiveStatus == "trialing" && trialEnd != nil {
		value := int(math.Ceil(trialEnd.Sub(now).Hours() / 24))
		daysRemaining = &value
	}
	return &SubscriptionContext{
		Status: effectiveStatus, Plan: plan, TrialEndsAt: trialEnd,
		DaysRemaining: daysRemaining, IsActive: isActive, IsExpired: isExpired,
		IsInGracePeriod: inGrace, GracePeriodEndsAt: graceEnd, ExpiryDate: expiryDate,
		WasTrialing: rawStatus == "trialing", CanAccessBookings: isActive || inGrace,
		CanAccessRegistrations: isActive || inGrace, CanAccessCalendar: isActive || inGrace,
		ShowTrialBanner:   effectiveStatus == "trialing" && daysRemaining != nil && *daysRemaining <= 7,
		ShowExpiredBanner: isExpired, CancelAt: cancelAt,
	}
}

func contextPermissions(role *string, membershipAllowed bool, subscription *SubscriptionContext) []string {
	permissions := []string{}
	if !membershipAllowed || role == nil {
		return permissions
	}
	if subscription != nil && subscription.CanAccessBookings {
		permissions = append(permissions, "bookings:access")
	}
	if subscription != nil && subscription.CanAccessRegistrations {
		permissions = append(permissions, "registrations:access")
	}
	if subscription != nil && subscription.CanAccessCalendar {
		permissions = append(permissions, "calendar:access")
	}
	if *role == "owner" || *role == "admin" {
		if subscription != nil && subscription.CanAccessBookings {
			permissions = append(permissions, "settings:manage", "team:manage")
		}
		permissions = append(permissions, "billing:manage")
	}
	if *role == "owner" && subscription != nil && subscription.CanAccessCalendar {
		permissions = append(permissions, "connected_apps:manage")
	}
	return permissions
}

func nullableTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

package server

import (
	"time"

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
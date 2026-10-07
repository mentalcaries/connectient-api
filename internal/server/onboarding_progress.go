package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

var walkthroughKeys = []string{"setup", "appointments", "create-appointment", "registrations"}

type onboardingProgressTimestamps struct {
	SeenAt      *time.Time `json:"seen_at"`
	DismissedAt *time.Time `json:"dismissed_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type patchOnboardingProgressInput struct {
	Seen      bool
	Dismissed bool
	Completed bool
}

func (s *Server) handlerGetOnboardingProgress(c *gin.Context) {
	setPrivateNoStore(c)
	claims := c.MustGet("claims").(TokenClaims)
	rows, err := s.DBQuery.ListOnboardingProgress(c, claims.ID)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch onboarding progress", err)
		return
	}
	progress := make(map[string]any, len(walkthroughKeys))
	for _, key := range walkthroughKeys {
		progress[key] = nil
	}
	for _, row := range rows {
		if !slices.Contains(walkthroughKeys, row.WalkthroughKey) {
			continue
		}
		progress[row.WalkthroughKey] = progressTimestamps(row.SeenAt, row.DismissedAt, row.CompletedAt)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": progress})
}

func (s *Server) handlerPatchOnboardingProgress(c *gin.Context) {
	setPrivateNoStore(c)
	claims := c.MustGet("claims").(TokenClaims)
	key := c.Param("key")
	if !slices.Contains(walkthroughKeys, key) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid walkthrough key"})
		return
	}
	input, err := decodePatchOnboardingProgress(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	row, err := s.DBQuery.UpsertOnboardingProgress(c, db.UpsertOnboardingProgressParams{
		UserID: claims.ID, WalkthroughKey: key, SetSeen: input.Seen,
		SetDismissed: input.Dismissed, SetCompleted: input.Completed,
	})
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to update onboarding progress", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"walkthrough_key": row.WalkthroughKey,
		"seen_at":         nullableTimestamp(row.SeenAt),
		"dismissed_at":    nullableTimestamp(row.DismissedAt),
		"completed_at":    nullableTimestamp(row.CompletedAt),
	}})
}

func decodePatchOnboardingProgress(body io.Reader) (patchOnboardingProgressInput, error) {
	var raw json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&raw); err != nil || ensureJSONEnd(decoder) != nil {
		return patchOnboardingProgressInput{}, errors.New("Invalid request body")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return patchOnboardingProgressInput{}, errors.New("Nothing to update")
	}
	input := patchOnboardingProgressInput{
		Seen:      rawJSONIsTrue(fields["seen"]),
		Dismissed: rawJSONIsTrue(fields["dismissed"]),
		Completed: rawJSONIsTrue(fields["completed"]),
	}
	if !input.Seen && !input.Dismissed && !input.Completed {
		return input, errors.New("Nothing to update")
	}
	return input, nil
}

func rawJSONIsTrue(raw json.RawMessage) bool {
	var value bool
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value
}

func progressTimestamps(seen, dismissed, completed pgtype.Timestamptz) onboardingProgressTimestamps {
	return onboardingProgressTimestamps{
		SeenAt: nullableTimestamp(seen), DismissedAt: nullableTimestamp(dismissed),
		CompletedAt: nullableTimestamp(completed),
	}
}

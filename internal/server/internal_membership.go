package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handlerInternalMembershipLookup(c *gin.Context) {
	setPrivateNoStore(c)
	expected := os.Getenv("AUTH_PROFILE_SERVICE_TOKEN")
	provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if expected == "" {
		respondWithError(c, http.StatusServiceUnavailable, "membership lookup is unavailable", nil)
		return
	}
	if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		respondWithError(c, http.StatusUnauthorized, "authorization required", nil)
		return
	}

	var body struct {
		UserID string `json:"userId"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || ensureJSONEnd(decoder) != nil {
		respondWithError(c, http.StatusBadRequest, "invalid request body", nil)
		return
	}
	userID, err := uuid.Parse(body.UserID)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "invalid user ID", nil)
		return
	}
	user, err := s.DBQuery.GetUserAuthorization(c, userID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !membershipAllowed(user) {
		respondWithError(c, http.StatusNotFound, "membership not found", nil)
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not load membership", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"membership": gin.H{
		"practice_id": user.PracticeID,
		"role":        user.Role,
	}})
}

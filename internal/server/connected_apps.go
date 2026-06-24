package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ConnectedApp struct {
	Id                    uuid.UUID `json:"id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	PracticeID            uuid.UUID `json:"practice_id"`
	Provider              string    `json:"provider"`
	ConnectedAccountEmail *string   `json:"connected_account_email,omitempty"`
	AccessToken           *string   `json:"access_token,omitempty"`
	RefreshToken          *string   `json:"refresh_token,omitempty"`
	TokenExpiresAt        time.Time `json:"token_expires_at"`
	IsConnected           bool      `json:"is_connected"`
	LastError             *string   `json:"last_error,omitempty"`
}

func (s *Server) handlerGetConnectedApps(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)

	dbConnectedApps, err := s.DBQuery.GetConnectedApps(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "could not get connected apps", err)
	}

	connectedApps := []ConnectedApp{}
	for _, dbConnectedApp := range dbConnectedApps {
		connectedApps = append(connectedApps, ConnectedApp{
			Id:                    dbConnectedApp.ID,
			CreatedAt:             dbConnectedApp.CreatedAt,
			UpdatedAt:             dbConnectedApp.UpdatedAt,
			PracticeID:            dbConnectedApp.PracticeID,
			Provider:              dbConnectedApp.Provider,
			ConnectedAccountEmail: dbConnectedApp.ConnectedAccountEmail,
			AccessToken:           dbConnectedApp.AccessToken,
			RefreshToken:          dbConnectedApp.AccessToken,
			TokenExpiresAt:        *dbConnectedApp.TokenExpiresAt,
			IsConnected:           dbConnectedApp.IsConnected,
			LastError:             dbConnectedApp.LastError,
		})
	}

	c.JSON(http.StatusOK, connectedApps)
}

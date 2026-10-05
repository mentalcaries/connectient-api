package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ConnectedApp struct {
	Provider              string  `json:"provider"`
	ConnectedAccountEmail *string `json:"connected_account_email"`
	Connected             bool    `json:"connected"`
}

func (s *Server) handlerGetConnectedApps(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)

	dbConnectedApps, err := s.DBQuery.GetConnectedApps(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "could not get connected apps", err)
		return
	}

	connectedApps := []ConnectedApp{}
	for _, dbConnectedApp := range dbConnectedApps {
		connectedApps = append(connectedApps, ConnectedApp{
			Provider:              dbConnectedApp.Provider,
			ConnectedAccountEmail: dbConnectedApp.ConnectedAccountEmail,
			Connected:             dbConnectedApp.IsConnected,
		})
	}

	c.JSON(http.StatusOK, connectedApps)
}

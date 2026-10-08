package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handlerGetPracticeSubscription(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	row, err := s.DBQuery.GetPracticeSubscription(c, *user.PracticeId)
	if errors.Is(err, pgx.ErrNoRows) {
		respondWithError(c, http.StatusNotFound, "practice not found", nil)
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "could not load subscription", err)
		return
	}
	subscription := computeSubscriptionContext(
		row.Status, row.Plan,
		nullableTimestamp(row.TrialEnd),
		nullableTimestamp(row.PeriodEnd),
		nullableTimestamp(row.CancelAt),
		time.Now(),
	)
	c.JSON(http.StatusOK, gin.H{"subscription": subscription})
}

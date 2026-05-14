package server

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func parseId(c *gin.Context, param string) (uuid.UUID, error) {
	requestParam := c.Param(param)
	if requestParam == "" {
		return uuid.UUID{}, errors.New("missing or invalid ID")
	}
	id, err := uuid.Parse(requestParam)
	if err != nil {
		return uuid.UUID{}, err
	}
	return id, nil
}

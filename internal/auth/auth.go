package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

type User struct {
	ID         string
	Email      string
	Name       string
	PracticeId string
	Role       string
}

var (
	ErrMissingUserId = errors.New("missing user id")
)

func UserFromRequest(c *gin.Context) (User, error) {
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if frontendBaseUrl == "" {
		return User{}, errors.New("FRONTEND_BASE_URL not configured")
	}

	keyset, err := jwk.Fetch(c.Request.Context(), frontendBaseUrl+"/api/auth/jwks")
	if err != nil {
		return User{}, fmt.Errorf("fetch jwks: %w", err)
	}

	token, err := jwt.ParseRequest(c.Request, jwt.WithKeySet(keyset))
	if err != nil {
		return User{}, fmt.Errorf("parse request: %w", err)
	}

	userId, ok := token.Subject()
	if !ok {
		return User{}, ErrMissingUserId
	}
	return User{
		ID: userId,
	}, nil
}

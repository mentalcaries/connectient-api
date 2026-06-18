package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func MakeToken() string {
	token := make([]byte, 32)
	rand.Read(token)
	return hex.EncodeToString(token)
}

type AuthUser struct {
	ID         uuid.UUID  `json:"id"`
	Email      *string    `json:"email"`
	Name       string     `json:"name"`
	PracticeId *uuid.UUID `json:"practice_id"`
	Role       *string    `json:"role"`
}

type TokenClaims struct {
	ID uuid.UUID
	Email  string
}

var (
	ErrMissingUserId = errors.New("missing user id")
)

func (s *Server) ClaimsFromRequest(c *gin.Context) (TokenClaims, error) {
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
	if frontendBaseUrl == "" {
		return TokenClaims{}, errors.New("FRONTEND_BASE_URL not configured")
	}

	keyset, err := jwk.Fetch(c.Request.Context(), frontendBaseUrl+"/api/auth/jwks")
	if err != nil {
		return TokenClaims{}, fmt.Errorf("fetch jwks: %w", err)
	}

	token, err := jwt.ParseRequest(c.Request, jwt.WithKeySet(keyset))
	if err != nil {
		return TokenClaims{}, fmt.Errorf("parse request: %w", err)
	}

	userIdStr, ok := token.Subject()
	if !ok {
		return TokenClaims{}, ErrMissingUserId
	}

	userId, err := uuid.Parse(userIdStr)
	if err != nil {
		return TokenClaims{}, fmt.Errorf("parse user id: %w", err)
	}

	var email string
	if err := token.Get("email", &email); err != nil {
		email = ""
	}
	return TokenClaims{ID: userId, Email: email}, nil
}

func (s *Server) UserFromRequest(c *gin.Context) (AuthUser, error) {
	claims, err := s.ClaimsFromRequest(c)
	if err != nil {
		return AuthUser{}, err
	}

	user, err := s.DBQuery.GetUser(c, claims.ID)
	if err != nil {
		return AuthUser{}, fmt.Errorf("get user: %w", err)
	}

	return AuthUser{
		ID:         user.ID,
		Email:      user.Email,
		Name:       user.FirstName + " " + user.LastName,
		PracticeId: user.PracticeID,
		Role:       user.Role,
	}, nil
}

// func (s *Server) UserFromRequest(c *gin.Context) (AuthUser, error) {
// 	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")
// 	if frontendBaseUrl == "" {
// 		return AuthUser{}, errors.New("FRONTEND_BASE_URL not configured")
// 	}

// 	keyset, err := jwk.Fetch(c.Request.Context(), frontendBaseUrl+"/api/auth/jwks")
// 	if err != nil {
// 		return AuthUser{}, fmt.Errorf("fetch jwks: %w", err)
// 	}

// 	token, err := jwt.ParseRequest(c.Request, jwt.WithKeySet(keyset))
// 	if err != nil {
// 		return AuthUser{}, fmt.Errorf("parse request: %w", err)
// 	}

// 	userIdStr, ok := token.Subject()
// 	if !ok {
// 		return AuthUser{}, ErrMissingUserId
// 	}

// 	userId, err := uuid.Parse(userIdStr)

// 	user, err := s.DBQuery.GetUser(c, userId)
// 	if err != nil {

// 	}

// 	return AuthUser{
// 		ID:         user.ID,
// 		Email:      user.Email,
// 		Name:       user.FirstName + " " + user.LastName,
// 		PracticeId: user.PracticeID,
// 		Role:       user.Role,
// 	}, nil
// }

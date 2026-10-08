package server

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var RESERVED_CODES = []string{
	"contact",
	"hipaa",
	"pricing",
	"privacy",
	"status",
	"terms",
	"access-revoked",
	"admin",
	"api",
	"invite",
	"register",
	"login",
	"reset",
	"signup",
	"resources",
}

var (
	nonAlphanumeric            = regexp.MustCompile(`[^a-z0-9\s]`)
	whitespace                 = regexp.MustCompile(`\s+`)
	multipleDashes             = regexp.MustCompile(`-+`)
	nonAlphanumericWithHyphens = regexp.MustCompile(`[^a-z0-9\s-]`)
)

func generateSlug(name string, allowHyphens bool) string {
	name = strings.ToLower(name)
	if allowHyphens {
		name = nonAlphanumericWithHyphens.ReplaceAllString(name, "")
	} else {
		name = nonAlphanumeric.ReplaceAllString(name, "")

	}
	name = whitespace.ReplaceAllString(name, "-")
	name = multipleDashes.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	return name
}

func truncateToSegments(slug string, maxSegments int) string {
	if maxSegments == 0 {
		maxSegments = 3
	}
	segments := strings.Split(slug, "-")
	if len(segments) <= maxSegments {
		return slug
	}
	return strings.Join(segments[:maxSegments], "-")
}

func (s *Server) practiceCodeExists(c *gin.Context, slug string) (bool, error) {
	_, err := s.DBQuery.CheckPracticeCodeExists(c, slug)

	if err == pgx.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Server) handlerSuggestPracticeCode(c *gin.Context) {
	setPrivateNoStore(c)
	name := c.Query("name")
	if strings.TrimSpace(name) == "" {
		respondWithError(c, http.StatusBadRequest, "Name is required", nil)
		return
	}

	slug := generateSlug(name, false)
	if len(slug) < 3 {
		respondWithError(c, http.StatusBadRequest, "Generated code is too short", nil)
		return
	}
	slug = truncateToSegments(slug, 3)

	codeExists, err := s.practiceCodeExists(c, slug)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Unable to check existing code", err)
		return
	}
	for suffix := 2; codeExists && suffix <= 99; suffix++ {
		updatedSlug := fmt.Sprintf("%s-%d", slug, suffix)
		codeExists, err = s.practiceCodeExists(c, updatedSlug)
		if err != nil {
			respondWithError(c, http.StatusInternalServerError, "Unable to check existing code", err)
			return
		}
		if codeExists {
			continue
		} else {
			slug = updatedSlug
		}
	}
	if codeExists {
		randomSuffix, err := rand.Int(rand.Reader, big.NewInt(1000))
		if err != nil {
			respondWithError(c, http.StatusInternalServerError, "Unable to generate code", err)
			return
		}
		slug = fmt.Sprintf("%s-%d", slug, randomSuffix.Int64())
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "suggestion": slug})
}

func (s *Server) handlerCheckCodeAvailability(c *gin.Context) {
	setPrivateNoStore(c)
	code := c.Query("code")
	if strings.TrimSpace(code) == "" {
		respondWithError(c, http.StatusBadRequest, "Code is required", nil)
		return
	}

	sanitizedCode := generateSlug(code, true)
	if len(sanitizedCode) < 3 {
		respondWithError(c, http.StatusBadRequest, "Code must be at least 3 characters after sanitization", nil)
		return
	}

	if slices.Contains(RESERVED_CODES, sanitizedCode) {
		c.JSON(http.StatusOK, gin.H{"success": true, "available": false, "code": sanitizedCode})
		return
	}

	codeExists, err := s.practiceCodeExists(c, sanitizedCode)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Unable to check existing code", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "available": !codeExists, "code": sanitizedCode})
}

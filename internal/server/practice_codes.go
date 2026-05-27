package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9\s]`)
	whitespace      = regexp.MustCompile(`\s+`)
	multipleDashes  = regexp.MustCompile(`-+`)
)

func generateSlug(name string) string {
	name = strings.ToLower(name)
	name = nonAlphanumeric.ReplaceAllString(name, "")
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
	name := c.Query("name")
	if strings.TrimSpace(name) == "" {
		respondWithError(c, http.StatusBadRequest, "Name is required", nil)
		return
	}

	slug := generateSlug(name)
	slug = truncateToSegments(slug, 3)

	codeExists, err := s.practiceCodeExists(c, slug)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Unable to check existing code", err)
		return
	}
	suffix := 2

	for codeExists {
		updatedSlug := fmt.Sprintf("%s-%d", slug, suffix)
		codeExists, err = s.practiceCodeExists(c, updatedSlug)
		if codeExists {
			suffix++
		} else {
			slug = updatedSlug
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "suggestion": slug})
}

package server

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func (s *Server) handlerUploadPracticeLogo(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if s.storage == nil {
		respondPracticeLogoError(c, http.StatusServiceUnavailable, "Logo storage is not configured", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProfileRequestSize)
	if err := c.Request.ParseMultipartForm(maxProfileRequestSize); err != nil {
		respondPracticeLogoError(c, http.StatusBadRequest, "Invalid multipart form", nil)
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondPracticeLogoError(c, http.StatusBadRequest, "No file provided", nil)
		return
	}
	defer file.Close()
	data, contentType, err := readValidatedLogo(file, header)
	if err != nil {
		respondPracticeLogoError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	current, err := s.DBQuery.GetPracticeProfile(c, *user.PracticeId)
	if err != nil {
		respondPracticeLogoError(c, http.StatusInternalServerError, "Failed to upload logo", err)
		return
	}
	key := fmt.Sprintf("%s/logo_%d", user.PracticeId.String(), time.Now().UnixNano())
	if err := s.storage.Put(c, key, contentType, bytes.NewReader(data), int64(len(data))); err != nil {
		respondPracticeLogoError(c, http.StatusInternalServerError, "Failed to upload logo", err)
		return
	}
	logoURL := s.storage.PublicURL(key)
	rows, err := s.DBQuery.UpdatePracticeLogo(c, db.UpdatePracticeLogoParams{Logo: &logoURL, ID: *user.PracticeId})
	if err != nil || rows != 1 {
		_ = s.storage.Delete(c, key)
		if err == nil {
			err = errors.New("practice not found")
		}
		respondPracticeLogoError(c, http.StatusInternalServerError, "Failed to upload logo", err)
		return
	}
	if current.Logo != nil {
		if oldKey, ok := s.storage.OwnedKey(*current.Logo, user.PracticeId.String()); ok {
			if err := s.storage.Delete(c, oldKey); err != nil {
				log.Printf("failed to delete replaced practice logo %q: %v", oldKey, err)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"url": logoURL})
}

func (s *Server) handlerDeletePracticeLogo(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if s.storage == nil {
		respondPracticeLogoError(c, http.StatusServiceUnavailable, "Logo storage is not configured", nil)
		return
	}
	current, err := s.DBQuery.GetPracticeProfile(c, *user.PracticeId)
	if err != nil {
		respondPracticeLogoError(c, http.StatusInternalServerError, "Failed to remove logo", err)
		return
	}
	rows, err := s.DBQuery.UpdatePracticeLogo(c, db.UpdatePracticeLogoParams{Logo: nil, ID: *user.PracticeId})
	if err != nil || rows != 1 {
		if err == nil {
			err = errors.New("practice not found")
		}
		respondPracticeLogoError(c, http.StatusInternalServerError, "Failed to remove logo", err)
		return
	}
	if current.Logo != nil {
		if key, ok := s.storage.OwnedKey(*current.Logo, user.PracticeId.String()); ok {
			if err := s.storage.Delete(c, key); err != nil {
				log.Printf("failed to delete removed practice logo %q: %v", key, err)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func respondPracticeLogoError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"error": message})
}

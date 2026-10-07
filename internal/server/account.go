package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type AccountResponse struct {
	ID                           uuid.UUID  `json:"id"`
	Email                        *string    `json:"email"`
	FirstName                    string     `json:"first_name"`
	LastName                     string     `json:"last_name"`
	MobilePhone                  *string    `json:"mobile_phone"`
	AvatarURL                    *string    `json:"avatar_url"`
	WhatsappNotificationsEnabled bool       `json:"whatsapp_notifications_enabled"`
	Role                         *string    `json:"role"`
	PracticeID                   *uuid.UUID `json:"practice_id"`
}

type accountPatchInput struct {
	FirstName                       string
	LastName                        string
	AvatarURL                       *string
	SetMobilePhone                  bool
	MobilePhone                     *string
	SetWhatsappNotificationsEnabled bool
	WhatsappNotificationsEnabled    bool
}

func (s *Server) handlerGetAccount(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	account, err := s.DBQuery.GetAccount(c, user.ID)
	if err != nil {
		respondAccountError(c, http.StatusInternalServerError, "Failed to fetch account", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": AccountResponse{
		ID: account.ID, Email: user.Email, FirstName: account.FirstName,
		LastName: account.LastName, MobilePhone: account.MobilePhone,
		AvatarURL:                    account.AvatarUrl,
		WhatsappNotificationsEnabled: account.WhatsappNotificationsEnabled,
		Role:                         account.Role, PracticeID: account.PracticeID,
	}})
}

func (s *Server) handlerPatchAccount(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	input, err := decodeAccountPatch(c.Request.Body)
	if err != nil {
		respondAccountError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if input.AvatarURL != nil {
		if s.storage == nil || user.PracticeId == nil {
			respondAccountError(c, http.StatusServiceUnavailable, "Avatar storage is not configured", nil)
			return
		}
		key, ok := s.storage.OwnedKey(*input.AvatarURL, user.PracticeId.String())
		expected := user.PracticeId.String() + "/avatars/" + user.ID.String()
		if !ok || key != expected {
			respondAccountError(c, http.StatusBadRequest, "Invalid avatar_url", nil)
			return
		}
	}
	rows, err := s.DBQuery.UpdateAccount(c, db.UpdateAccountParams{
		FirstName: input.FirstName, LastName: input.LastName, AvatarUrl: input.AvatarURL,
		SetMobilePhone: input.SetMobilePhone, MobilePhone: input.MobilePhone,
		SetWhatsappNotificationsEnabled: input.SetWhatsappNotificationsEnabled,
		WhatsappNotificationsEnabled:    input.WhatsappNotificationsEnabled, ID: user.ID,
	})
	if isUniqueViolation(err) {
		respondAccountError(c, http.StatusConflict, "Mobile phone is already in use", nil)
		return
	}
	if err != nil || rows != 1 {
		if err == nil {
			err = errors.New("account not found")
		}
		respondAccountError(c, http.StatusInternalServerError, "Failed to update account", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerUploadAvatar(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if s.storage == nil || user.PracticeId == nil {
		respondAccountError(c, http.StatusServiceUnavailable, "Avatar storage is not configured", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProfileRequestSize)
	if err := c.Request.ParseMultipartForm(maxProfileRequestSize); err != nil {
		respondAccountError(c, http.StatusBadRequest, "Invalid multipart form", nil)
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondAccountError(c, http.StatusBadRequest, "No file provided", nil)
		return
	}
	defer file.Close()
	data, contentType, err := readValidatedLogo(file, header)
	if err != nil {
		respondAccountError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	key := user.PracticeId.String() + "/avatars/" + user.ID.String()
	if err := s.storage.Put(c, key, contentType, bytes.NewReader(data), int64(len(data))); err != nil {
		respondAccountError(c, http.StatusInternalServerError, "Failed to upload avatar", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": s.storage.PublicURL(key)})
}

func (s *Server) handlerDeleteAvatar(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if s.storage == nil || user.PracticeId == nil {
		respondAccountError(c, http.StatusServiceUnavailable, "Avatar storage is not configured", nil)
		return
	}
	key := user.PracticeId.String() + "/avatars/" + user.ID.String()
	if err := s.storage.Delete(c, key); err != nil {
		respondAccountError(c, http.StatusInternalServerError, "Failed to remove avatar", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodeAccountPatch(body io.Reader) (accountPatchInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return accountPatchInput{}, errors.New("Invalid request")
	}
	firstName, firstOK := accountString(fields, "first_name")
	lastName, lastOK := accountString(fields, "last_name")
	if !firstOK || firstName == "" {
		return accountPatchInput{}, errors.New("First name is required")
	}
	if !lastOK || lastName == "" {
		return accountPatchInput{}, errors.New("Last name is required")
	}
	input := accountPatchInput{FirstName: firstName, LastName: lastName}
	if raw, ok := fields["avatar_url"]; ok && !isJSONNull(raw) {
		var value string
		if json.Unmarshal(raw, &value) != nil || value == "" {
			return accountPatchInput{}, errors.New("Invalid avatar_url")
		}
		input.AvatarURL = &value
	}
	if raw, ok := fields["mobile_phone"]; ok {
		input.SetMobilePhone = true
		if !isJSONNull(raw) {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return accountPatchInput{}, errors.New("Invalid mobile_phone")
			}
			input.MobilePhone = &value
		}
	}
	if raw, ok := fields["whatsapp_notifications_enabled"]; ok {
		var err error
		input.WhatsappNotificationsEnabled, err = decodeBoolean(raw, "whatsapp_notifications_enabled")
		if err != nil {
			return accountPatchInput{}, err
		}
		input.SetWhatsappNotificationsEnabled = true
	}
	return input, nil
}

func accountString(fields map[string]json.RawMessage, name string) (string, bool) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func respondAccountError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"error": message})
}

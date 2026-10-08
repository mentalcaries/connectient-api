package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type ProviderResponse struct {
	ID        uuid.UUID  `json:"id"`
	CreatedAt *time.Time `json:"created_at"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Title     *string    `json:"title"`
	Specialty string     `json:"specialty"`
	IsMain    bool       `json:"is_main"`
}

type createProviderInput struct {
	FirstName string
	LastName  string
	Title     *string
	Specialty string
	IsMain    bool
}

type patchProviderInput struct {
	params db.PatchProviderParams
}

func (input patchProviderInput) hasUpdates() bool {
	params := input.params
	return params.SetFirstName || params.SetLastName || params.SetTitle || params.SetSpecialty
}

func (s *Server) handlerGetPracticeProviders(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	rows, err := s.DBQuery.GetProvidersByPracticeID(c, *user.PracticeId)
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to fetch providers", err)
		return
	}
	providers := make([]ProviderResponse, 0, len(rows))
	for _, row := range rows {
		providers = append(providers, providerResponse(row.ID, row.CreatedAt, row.FirstName, row.LastName, row.Title, row.Specialty, row.IsMain))
	}
	c.JSON(http.StatusOK, gin.H{"data": providers})
}

func (s *Server) handlerCreatePracticeProvider(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	input, err := decodeCreateProvider(c.Request.Body)
	if err != nil {
		respondProviderError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	if _, err := queries.LockPracticeForProviderMutation(c, *user.PracticeId); err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
		return
	}
	existingProviders, err := queries.GetProvidersByPracticeID(c, *user.PracticeId)
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
		return
	}
	isMain := input.IsMain || len(existingProviders) == 0
	provider, err := queries.CreateProvider(c, db.CreateProviderParams{
		FirstName: input.FirstName, LastName: input.LastName,
		Title: input.Title, Specialty: input.Specialty,
	})
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
		return
	}
	if isMain {
		if err := queries.ClearMainProvider(c, *user.PracticeId); err != nil {
			respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
			return
		}
	}
	if _, err := queries.CreatePracticeProviderLink(c, db.CreatePracticeProviderLinkParams{
		PracticeID: *user.PracticeId, ProviderID: provider.ID, IsMain: isMain,
	}); err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to create provider", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": providerResponse(
		provider.ID, provider.CreatedAt, provider.FirstName, provider.LastName,
		provider.Title, provider.Specialty, isMain,
	)})
}

func (s *Server) handlerPatchPracticeProvider(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	providerID, ok := providerIDFromRequest(c)
	if !ok {
		return
	}
	input, err := decodePatchProvider(c.Request.Body)
	if err != nil {
		respondProviderError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !input.hasUpdates() {
		respondProviderError(c, http.StatusBadRequest, "No valid fields to update", nil)
		return
	}
	input.params.ProviderID = providerID
	input.params.PracticeID = *user.PracticeId
	rows, err := s.DBQuery.PatchProvider(c, input.params)
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update provider", err)
		return
	}
	if rows == 0 {
		respondProviderError(c, http.StatusNotFound, "Provider not found", nil)
		return
	}
	provider, err := s.DBQuery.GetProviderForMutation(c, db.GetProviderForMutationParams{
		PracticeID: *user.PracticeId, ProviderID: providerID,
	})
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update provider", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": providerResponse(
		provider.ID, provider.CreatedAt, provider.FirstName, provider.LastName,
		provider.Title, provider.Specialty, provider.IsMain,
	)})
}

func (s *Server) handlerSetMainPracticeProvider(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	providerID, ok := providerIDFromRequest(c)
	if !ok {
		return
	}
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update main provider", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	if _, err := queries.LockPracticeForProviderMutation(c, *user.PracticeId); err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update main provider", err)
		return
	}
	if _, err := queries.GetProviderForMutation(c, db.GetProviderForMutationParams{
		PracticeID: *user.PracticeId, ProviderID: providerID,
	}); errors.Is(err, pgx.ErrNoRows) {
		respondProviderError(c, http.StatusNotFound, "Provider not found", nil)
		return
	} else if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update main provider", err)
		return
	}
	if err := queries.ClearMainProvider(c, *user.PracticeId); err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update main provider", err)
		return
	}
	rows, err := queries.SetMainProvider(c, db.SetMainProviderParams{PracticeID: *user.PracticeId, ProviderID: providerID})
	if err != nil || rows != 1 {
		if err == nil {
			err = errors.New("provider link not found")
		}
		respondProviderError(c, http.StatusInternalServerError, "Failed to update main provider", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to update main provider", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerDeletePracticeProvider(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	providerID, ok := providerIDFromRequest(c)
	if !ok {
		return
	}
	provider, err := s.DBQuery.GetProviderForMutation(c, db.GetProviderForMutationParams{
		PracticeID: *user.PracticeId, ProviderID: providerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		respondProviderError(c, http.StatusNotFound, "Provider not found", nil)
		return
	}
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to remove provider", err)
		return
	}
	if provider.IsMain {
		respondProviderError(c, http.StatusBadRequest, "Main provider cannot be removed", nil)
		return
	}
	rows, err := s.DBQuery.DeletePracticeProviderLink(c, db.DeletePracticeProviderLinkParams{
		PracticeID: *user.PracticeId, ProviderID: providerID,
	})
	if err != nil {
		respondProviderError(c, http.StatusInternalServerError, "Failed to remove provider", err)
		return
	}
	if rows == 0 {
		respondProviderError(c, http.StatusNotFound, "Provider not found", nil)
		return
	}
	c.Status(http.StatusNoContent)
}

func decodeCreateProvider(body io.Reader) (createProviderInput, error) {
	fields, err := decodeProviderFields(body)
	if err != nil {
		return createProviderInput{}, err
	}
	firstName, firstOK := providerString(fields, "first_name")
	lastName, lastOK := providerString(fields, "last_name")
	if !firstOK || !lastOK || firstName == "" || lastName == "" {
		return createProviderInput{}, errors.New("First name and last name are required")
	}
	input := createProviderInput{FirstName: firstName, LastName: lastName, Specialty: "General"}
	if raw, ok := fields["title"]; ok && !isJSONNull(raw) {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return createProviderInput{}, errors.New("Invalid title")
		}
		value = strings.TrimSpace(value)
		if value != "" {
			input.Title = &value
		}
	}
	if raw, ok := fields["specialty"]; ok && !isJSONNull(raw) {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return createProviderInput{}, errors.New("Invalid specialty")
		}
		if value = strings.TrimSpace(value); value != "" {
			input.Specialty = value
		}
	}
	if raw, ok := fields["is_main"]; ok {
		input.IsMain, err = decodeBoolean(raw, "is_main")
		if err != nil {
			return createProviderInput{}, err
		}
	}
	return input, nil
}

func decodePatchProvider(body io.Reader) (patchProviderInput, error) {
	fields, err := decodeProviderFields(body)
	if err != nil {
		return patchProviderInput{}, err
	}
	var input patchProviderInput
	params := &input.params
	if _, ok := fields["first_name"]; ok {
		params.FirstName, ok = providerString(fields, "first_name")
		if !ok || params.FirstName == "" {
			return patchProviderInput{}, errors.New("Invalid first_name")
		}
		params.SetFirstName = true
	}
	if _, ok := fields["last_name"]; ok {
		params.LastName, ok = providerString(fields, "last_name")
		if !ok || params.LastName == "" {
			return patchProviderInput{}, errors.New("Invalid last_name")
		}
		params.SetLastName = true
	}
	if raw, ok := fields["title"]; ok {
		params.SetTitle = true
		if !isJSONNull(raw) {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return patchProviderInput{}, errors.New("Invalid title")
			}
			if value = strings.TrimSpace(value); value != "" {
				params.Title = &value
			}
		}
	}
	if raw, ok := fields["specialty"]; ok {
		params.SetSpecialty = true
		params.Specialty = "General"
		if !isJSONNull(raw) {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return patchProviderInput{}, errors.New("Invalid specialty")
			}
			if value = strings.TrimSpace(value); value != "" {
				params.Specialty = value
			}
		}
	}
	return input, nil
}

func providerString(fields map[string]json.RawMessage, name string) (string, bool) {
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

func decodeProviderFields(body io.Reader) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return nil, errors.New("Invalid request")
	}
	return fields, nil
}

func providerIDFromRequest(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondProviderError(c, http.StatusBadRequest, "Invalid provider ID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func providerResponse(id uuid.UUID, createdAt *time.Time, firstName, lastName string, title *string, specialty string, isMain bool) ProviderResponse {
	return ProviderResponse{
		ID: id, CreatedAt: createdAt, FirstName: firstName, LastName: lastName,
		Title: title, Specialty: specialty, IsMain: isMain,
	}
}

func respondProviderError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"error": message})
}

package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type createPracticeLocationInput struct {
	Name    string
	Address *string
}

type patchPracticeLocationInput struct {
	params db.PatchPracticeLocationParams
}

func (input patchPracticeLocationInput) hasUpdates() bool {
	params := input.params
	return params.SetName || params.SetAddress || params.SetIsActive || params.SetSortOrder ||
		params.SetDeletedAt || params.SetAvailableWeekdays
}

func (s *Server) handlerGetPracticeLocations(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	locations, err := s.DBQuery.GetPracticeLocationsByPracticeID(c, *user.PracticeId)
	if err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to fetch locations", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    practiceLocationResponses(locations),
	})
}

func (s *Server) handlerCreatePracticeLocation(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	input, err := decodeCreatePracticeLocation(c.Request.Body)
	if err != nil {
		respondLocationError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to create location", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	if _, err := queries.LockPracticeForLocationSort(c, *user.PracticeId); err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to create location", err)
		return
	}
	sortOrder, err := queries.GetNextPracticeLocationSortOrder(c, *user.PracticeId)
	if err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to create location", err)
		return
	}
	location, err := queries.CreatePracticeLocation(c, db.CreatePracticeLocationParams{
		PracticeID: *user.PracticeId,
		Name:       input.Name,
		Address:    input.Address,
		SortOrder:  sortOrder,
	})
	if err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to create location", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to create location", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    practiceLocationResponse(location),
	})
}

func (s *Server) handlerPatchPracticeLocation(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	locationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondLocationError(c, http.StatusBadRequest, "Invalid location ID", nil)
		return
	}
	input, err := decodePatchPracticeLocation(c.Request.Body)
	if err != nil {
		respondLocationError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !input.hasUpdates() {
		respondLocationError(c, http.StatusBadRequest, "No valid fields to update", nil)
		return
	}
	input.params.ID = locationID
	input.params.PracticeID = *user.PracticeId
	rows, err := s.DBQuery.PatchPracticeLocation(c, input.params)
	if err != nil {
		respondLocationError(c, http.StatusInternalServerError, "Failed to update location", err)
		return
	}
	if rows == 0 {
		respondLocationError(c, http.StatusNotFound, "Location not found", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodeCreatePracticeLocation(body io.Reader) (createPracticeLocationInput, error) {
	fields, err := decodeLocationFields(body)
	if err != nil {
		return createPracticeLocationInput{}, err
	}
	rawName, ok := fields["name"]
	if !ok {
		return createPracticeLocationInput{}, errors.New("Name is required")
	}
	var input createPracticeLocationInput
	if err := json.Unmarshal(rawName, &input.Name); err != nil || input.Name == "" {
		return createPracticeLocationInput{}, errors.New("Name is required")
	}
	if rawAddress, ok := fields["address"]; ok && !isJSONNull(rawAddress) {
		var address string
		if err := json.Unmarshal(rawAddress, &address); err != nil {
			return createPracticeLocationInput{}, errors.New("Invalid address")
		}
		if address != "" {
			input.Address = &address
		}
	}
	return input, nil
}

func decodePatchPracticeLocation(body io.Reader) (patchPracticeLocationInput, error) {
	fields, err := decodeLocationFields(body)
	if err != nil {
		return patchPracticeLocationInput{}, err
	}
	var input patchPracticeLocationInput
	params := &input.params
	if raw, ok := fields["name"]; ok {
		if isJSONNull(raw) || json.Unmarshal(raw, &params.Name) != nil {
			return patchPracticeLocationInput{}, errors.New("Invalid name")
		}
		params.SetName = true
	}
	if raw, ok := fields["address"]; ok {
		params.SetAddress = true
		if !isJSONNull(raw) {
			var address string
			if err := json.Unmarshal(raw, &address); err != nil {
				return patchPracticeLocationInput{}, errors.New("Invalid address")
			}
			params.Address = &address
		}
	}
	if raw, ok := fields["is_active"]; ok {
		params.IsActive, err = decodeBoolean(raw, "is_active")
		if err != nil {
			return patchPracticeLocationInput{}, err
		}
		params.SetIsActive = true
	}
	if raw, ok := fields["sort_order"]; ok {
		if isJSONNull(raw) || json.Unmarshal(raw, &params.SortOrder) != nil {
			return patchPracticeLocationInput{}, errors.New("Invalid sort_order")
		}
		params.SetSortOrder = true
	}
	if raw, ok := fields["deleted_at"]; ok {
		params.SetDeletedAt = true
		if !isJSONNull(raw) {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return patchPracticeLocationInput{}, errors.New("Invalid deleted_at")
			}
			deletedAt, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return patchPracticeLocationInput{}, errors.New("Invalid deleted_at")
			}
			params.DeletedAt = pgtype.Timestamptz{Time: deletedAt, Valid: true}
		}
	}
	if raw, ok := fields["available_weekdays"]; ok {
		params.AvailableWeekdays, err = normalizeAvailableWeekdays(raw)
		if err != nil {
			return patchPracticeLocationInput{}, err
		}
		params.SetAvailableWeekdays = true
	}
	return input, nil
}

func decodeLocationFields(body io.Reader) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return nil, errors.New("Invalid request")
	}
	return fields, nil
}

func practiceLocationResponse(row db.PracticeLocation) PracticeSettingsLocation {
	return PracticeSettingsLocation{
		ID: row.ID, CreatedAt: row.CreatedAt, DeletedAt: row.DeletedAt,
		PracticeID: row.PracticeID, Name: row.Name, Address: row.Address,
		IsActive: row.IsActive, SortOrder: row.SortOrder,
		AvailableWeekdays: row.AvailableWeekdays,
	}
}

func respondLocationError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}

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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type ProcedureType struct {
	ID         uuid.UUID  `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	DeletedAt  *time.Time `json:"deleted_at"`
	PracticeID uuid.UUID  `json:"practice_id"`
	Name       string     `json:"name"`
	Value      string     `json:"value"`
	IsActive   bool       `json:"is_active"`
	IsDefault  bool       `json:"is_default"`
	IsPrimary  bool       `json:"is_primary"`
	SortOrder  int        `json:"sort_order"`
}

type DefaultProcedureType struct {
	Name      string
	Value     string
	SortOrder int
	IsPrimary bool
}

type DefaultPracticeSettings struct {
	DentalHistoryEnabled        bool
	TMJHistoryEnabled           bool
	MultipleLocationsEnabled    bool
	CustomFormSections          json.RawMessage
	PhysiotherapyHistoryEnabled bool
	OptometryHistoryEnabled     bool
}

type createProcedureTypeInput struct {
	Name      string
	Value     string
	IsPrimary bool
}

type patchProcedureTypeInput struct {
	params db.PatchProcedureTypeParams
}

func (input patchProcedureTypeInput) hasUpdates() bool {
	return input.params.SetName || input.params.SetIsActive || input.params.SetSortOrder
}

var DefaultProcedureTypes = map[string][]DefaultProcedureType{
	"dental": {
		{Name: "Consultation", Value: "consultation", SortOrder: 1, IsPrimary: true},
		{Name: "Cleaning/Polishing", Value: "cleaning", SortOrder: 2, IsPrimary: false},
		{Name: "Extraction", Value: "extraction", SortOrder: 3, IsPrimary: false},
		{Name: "Filling", Value: "filling", SortOrder: 4, IsPrimary: false},
		{Name: "Something Else", Value: "other", SortOrder: 5, IsPrimary: false},
	},
	"medical": {
		{Name: "Consultation", Value: "consultation", SortOrder: 1, IsPrimary: true},
		{Name: "Review", Value: "review", SortOrder: 2, IsPrimary: false},
	},
	"optometry": {
		{Name: "Routine Eye Examination", Value: "routine_eye_exam", SortOrder: 1, IsPrimary: true},
		{Name: "Contact Lens Fitting", Value: "contact_lens_fitting", SortOrder: 2, IsPrimary: false},
		{Name: "Glasses Prescription", Value: "glasses_prescription", SortOrder: 3, IsPrimary: false},
		{Name: "Emergency Eye Consultation", Value: "emergency_eye_consult", SortOrder: 4, IsPrimary: false},
	},
	"physiotherapy": {
		{Name: "Initial Assessment / Consultation", Value: "initial_assessment", SortOrder: 1, IsPrimary: true},
		{Name: "Follow-up Session", Value: "follow_up", SortOrder: 2, IsPrimary: false},
		{Name: "Manual Therapy", Value: "manual_therapy", SortOrder: 3, IsPrimary: false},
		{Name: "Exercise Therapy", Value: "exercise_therapy", SortOrder: 4, IsPrimary: false},
	},
}

var DefaultSettingsByCategory = map[string]DefaultPracticeSettings{
	"dental":        {DentalHistoryEnabled: true},
	"medical":       {},
	"optometry":     {OptometryHistoryEnabled: true},
	"physiotherapy": {PhysiotherapyHistoryEnabled: true},
}

func (s *Server) handlerGetPracticeProcedures(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)

	dbProcedures, err := s.DBQuery.GetProcedureTypesByPracticeID(c, *user.PracticeId)
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to fetch procedure types", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": procedureTypeResponses(dbProcedures)})
}

func (s *Server) handlerCreateProcedureType(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	input, err := decodeCreateProcedureType(c.Request.Body)
	if err != nil {
		respondProcedureTypeError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to create procedure type", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	if _, err := queries.LockPracticeForProcedureTypeSort(c, *user.PracticeId); err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to create procedure type", err)
		return
	}
	sortOrder, err := queries.GetNextProcedureTypeSortOrder(c, *user.PracticeId)
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to create procedure type", err)
		return
	}
	if input.IsPrimary {
		if err := queries.ClearPrimaryProcedureType(c, *user.PracticeId); err != nil {
			respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to create procedure type", err)
			return
		}
	}
	procedure, err := queries.CreateProcedureType(c, db.CreateProcedureTypeParams{
		PracticeID: *user.PracticeId,
		Name:       input.Name,
		Value:      input.Value,
		IsActive:   true,
		IsDefault:  false,
		IsPrimary:  input.IsPrimary,
		SortOrder:  sortOrder,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondProcedureTypeError(c, http.StatusConflict, "Procedure type with this value already exists", nil)
			return
		}
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to create procedure type", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to create procedure type", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": procedureTypeResponse(procedure)})
}

func (s *Server) handlerPatchProcedureType(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	procedureID, ok := procedureTypeID(c)
	if !ok {
		return
	}
	existing, err := s.DBQuery.GetProcedureTypeForMutation(c, db.GetProcedureTypeForMutationParams{
		ID: procedureID, PracticeID: *user.PracticeId,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		respondProcedureTypeError(c, http.StatusNotFound, "Procedure type not found", nil)
		return
	}
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to update procedure type", err)
		return
	}
	input, err := decodePatchProcedureType(c.Request.Body)
	if err != nil {
		respondProcedureTypeError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !input.hasUpdates() {
		respondProcedureTypeError(c, http.StatusBadRequest, "No valid fields to update", nil)
		return
	}
	if existing.IsDefault && input.params.SetName {
		respondProcedureTypeError(c, http.StatusForbidden, "Cannot edit name of default procedure types", nil)
		return
	}
	input.params.ID = procedureID
	input.params.PracticeID = *user.PracticeId
	rows, err := s.DBQuery.PatchProcedureType(c, input.params)
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to update procedure type", err)
		return
	}
	if rows == 0 {
		respondProcedureTypeError(c, http.StatusNotFound, "Procedure type not found", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerDeleteProcedureType(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	procedureID, ok := procedureTypeID(c)
	if !ok {
		return
	}
	existing, err := s.DBQuery.GetProcedureTypeForMutation(c, db.GetProcedureTypeForMutationParams{
		ID: procedureID, PracticeID: *user.PracticeId,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		respondProcedureTypeError(c, http.StatusNotFound, "Procedure type not found", nil)
		return
	}
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to delete procedure type", err)
		return
	}
	if existing.IsDefault {
		respondProcedureTypeError(c, http.StatusForbidden, "Cannot delete default procedure types", nil)
		return
	}
	rows, err := s.DBQuery.DeleteProcedureType(c, db.DeleteProcedureTypeParams{ID: procedureID, PracticeID: *user.PracticeId})
	if err != nil {
		respondProcedureTypeError(c, http.StatusInternalServerError, "Failed to delete procedure type", err)
		return
	}
	if rows == 0 {
		respondProcedureTypeError(c, http.StatusNotFound, "Procedure type not found", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodeCreateProcedureType(body io.Reader) (createProcedureTypeInput, error) {
	fields, err := decodeProcedureTypeFields(body)
	if err != nil {
		return createProcedureTypeInput{}, err
	}
	var input createProcedureTypeInput
	name, nameOK := fields["name"]
	value, valueOK := fields["value"]
	if !nameOK || !valueOK || isJSONNull(name) || isJSONNull(value) ||
		json.Unmarshal(name, &input.Name) != nil || json.Unmarshal(value, &input.Value) != nil ||
		input.Name == "" || input.Value == "" {
		return createProcedureTypeInput{}, errors.New("Name and value are required")
	}
	if raw, ok := fields["is_primary"]; ok {
		input.IsPrimary, err = decodeBoolean(raw, "is_primary")
		if err != nil {
			return createProcedureTypeInput{}, err
		}
	}
	return input, nil
}

func decodePatchProcedureType(body io.Reader) (patchProcedureTypeInput, error) {
	fields, err := decodeProcedureTypeFields(body)
	if err != nil {
		return patchProcedureTypeInput{}, err
	}
	var input patchProcedureTypeInput
	params := &input.params
	if raw, ok := fields["name"]; ok {
		if isJSONNull(raw) || json.Unmarshal(raw, &params.Name) != nil {
			return patchProcedureTypeInput{}, errors.New("Invalid name")
		}
		params.SetName = true
	}
	if raw, ok := fields["is_active"]; ok {
		params.IsActive, err = decodeBoolean(raw, "is_active")
		if err != nil {
			return patchProcedureTypeInput{}, err
		}
		params.SetIsActive = true
	}
	if raw, ok := fields["sort_order"]; ok {
		if isJSONNull(raw) || json.Unmarshal(raw, &params.SortOrder) != nil {
			return patchProcedureTypeInput{}, errors.New("Invalid sort_order")
		}
		params.SetSortOrder = true
	}
	return input, nil
}

func decodeProcedureTypeFields(body io.Reader) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return nil, errors.New("Invalid request")
	}
	return fields, nil
}

func procedureTypeID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondProcedureTypeError(c, http.StatusBadRequest, "Invalid procedure type ID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func procedureTypeResponses(rows []db.ProcedureType) []ProcedureType {
	result := make([]ProcedureType, 0, len(rows))
	for _, row := range rows {
		result = append(result, procedureTypeResponse(row))
	}
	return result
}

func procedureTypeResponse(row db.ProcedureType) ProcedureType {
	return ProcedureType{
		ID: row.ID, CreatedAt: row.CreatedAt, DeletedAt: row.DeletedAt,
		PracticeID: row.PracticeID, Name: row.Name, Value: row.Value,
		IsActive: row.IsActive, IsDefault: row.IsDefault, IsPrimary: row.IsPrimary,
		SortOrder: int(row.SortOrder),
	}
}

func respondProcedureTypeError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}

package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

var practiceSeatLimits = map[string]int32{"essential": 1, "pro": 3, "practice": 10}

type patchPracticeUserInput struct {
	SetRole     bool
	Role        string
	SetIsActive bool
	IsActive    bool
}

func (s *Server) handlerListPracticeUsers(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	rows, err := s.DBQuery.ListPracticeUsers(c, user.PracticeId)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to fetch users", err)
		return
	}
	data := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		isOwner := row.Role != nil && *row.Role == "owner"
		data = append(data, gin.H{
			"id": row.ID, "first_name": row.FirstName, "last_name": row.LastName,
			"email": row.Email, "role": row.Role, "org_role": row.OrgRole,
			"is_active": row.IsActive, "created_at": row.CreatedAt, "is_owner": isOwner,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (s *Server) handlerGetPracticeSeats(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	usage, err := s.practiceSeatUsage(c, user.PracticeId)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to fetch seat usage", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": usage})
}

func (s *Server) handlerPatchPracticeUser(c *gin.Context) {
	setPrivateNoStore(c)
	caller := c.MustGet("user").(AuthUser)
	targetID, ok := parseTeamUserID(c)
	if !ok {
		return
	}
	if caller.ID == targetID {
		respondTeamError(c, http.StatusForbidden, "Cannot modify your own record", nil)
		return
	}
	input, err := decodePatchPracticeUser(c.Request.Body)
	if err != nil {
		respondTeamError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to update user", err)
		return
	}
	defer tx.Rollback(c)
	if _, err := tx.Exec(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, caller.PracticeId.String()+":seats"); err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to update user", err)
		return
	}
	queries := s.DBQuery.WithTx(tx)
	target, err := queries.GetPracticeUserMutationState(c, db.GetPracticeUserMutationStateParams{ID: targetID, PracticeID: caller.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondTeamError(c, http.StatusNotFound, "User not found", nil)
		return
	}
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to update user", err)
		return
	}
	if target.Role != nil && *target.Role == "owner" {
		respondTeamError(c, http.StatusForbidden, "Cannot modify the practice owner", nil)
		return
	}
	if input.SetIsActive && input.IsActive && !target.IsActive {
		usage, err := practiceSeatUsageWithQueries(c, queries, caller.PracticeId)
		if err != nil {
			respondTeamError(c, http.StatusInternalServerError, "Failed to update user", err)
			return
		}
		if usage.AtLimit {
			respondSeatLimit(c, usage)
			return
		}
	}
	rows, err := queries.PatchPracticeUser(c, db.PatchPracticeUserParams{
		SetRole: input.SetRole, Role: input.Role, SetIsActive: input.SetIsActive,
		IsActive: input.IsActive, ID: targetID, PracticeID: caller.PracticeId,
	})
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to update user", err)
		return
	}
	if rows != 1 {
		respondTeamError(c, http.StatusNotFound, "User not found", nil)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to update user", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerDeletePracticeUser(c *gin.Context) {
	setPrivateNoStore(c)
	caller := c.MustGet("user").(AuthUser)
	targetID, ok := parseTeamUserID(c)
	if !ok {
		return
	}
	if caller.ID == targetID {
		respondTeamError(c, http.StatusForbidden, "Cannot delete your own record", nil)
		return
	}
	target, err := s.DBQuery.GetPracticeUserMutationState(c, db.GetPracticeUserMutationStateParams{ID: targetID, PracticeID: caller.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondTeamError(c, http.StatusNotFound, "User not found", nil)
		return
	}
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to delete user", err)
		return
	}
	if target.Role != nil && *target.Role == "owner" {
		respondTeamError(c, http.StatusForbidden, "Cannot delete the practice owner", nil)
		return
	}
	rows, err := s.DBQuery.SoftDeletePracticeUser(c, db.SoftDeletePracticeUserParams{ID: targetID, PracticeID: caller.PracticeId})
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to delete user", err)
		return
	}
	if rows != 1 {
		respondTeamError(c, http.StatusNotFound, "User not found", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type PracticeSeatUsage struct {
	Used    int32  `json:"used"`
	Limit   int32  `json:"limit"`
	Plan    string `json:"plan"`
	AtLimit bool   `json:"atLimit"`
}

func (s *Server) practiceSeatUsage(c *gin.Context, practiceID *uuid.UUID) (PracticeSeatUsage, error) {
	return practiceSeatUsageWithQueries(c, s.DBQuery, practiceID)
}

func practiceSeatUsageWithQueries(c *gin.Context, queries *db.Queries, practiceID *uuid.UUID) (PracticeSeatUsage, error) {
	row, err := queries.GetPracticeSeatUsage(c, practiceID)
	if err != nil {
		return PracticeSeatUsage{}, err
	}
	limit, ok := practiceSeatLimits[row.Plan]
	if !ok {
		row.Plan, limit = "pro", practiceSeatLimits["pro"]
	}
	return PracticeSeatUsage{Used: row.Used, Limit: limit, Plan: row.Plan, AtLimit: row.Used >= limit}, nil
}

func decodePatchPracticeUser(body io.Reader) (patchPracticeUserInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return patchPracticeUserInput{}, errors.New("Invalid request")
	}
	var input patchPracticeUserInput
	if raw, ok := fields["role"]; ok {
		if json.Unmarshal(raw, &input.Role) != nil || (input.Role != "admin" && input.Role != "staff") {
			return input, errors.New("Invalid role")
		}
		input.SetRole = true
	}
	if raw, ok := fields["is_active"]; ok {
		value, err := decodeBoolean(raw, "is_active")
		if err != nil {
			return input, err
		}
		input.SetIsActive, input.IsActive = true, value
	}
	if !input.SetRole && !input.SetIsActive {
		return input, errors.New("No valid fields to update")
	}
	return input, nil
}

func parseTeamUserID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondTeamError(c, http.StatusBadRequest, "Invalid user ID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func respondSeatLimit(c *gin.Context, usage PracticeSeatUsage) {
	c.JSON(http.StatusForbidden, gin.H{
		"error": "seat_limit_reached", "message": "Upgrade your plan to add more users.",
		"used": usage.Used, "limit": usage.Limit,
	})
}

func respondTeamError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	c.JSON(status, gin.H{"success": false, "error": message})
}

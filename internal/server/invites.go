package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const inviteTokenLifetime = 7 * 24 * time.Hour

type TeamInviteNotification struct {
	Email        string
	FirstName    string
	PracticeName string
	InviterName  string
	Role         string
	Link         string
}

type TeamInviteNotifier interface {
	SendTeamInvite(context.Context, TeamInviteNotification) (RegistrationDeliveryResult, error)
}

type createPracticeInviteInput struct {
	FirstName string
	LastName  string
	Email     string
	Role      string
	OrgRole   *string
}

func (s *Server) handlerListPracticeInvites(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	rows, err := s.DBQuery.ListPracticeInvites(c, *user.PracticeId)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to fetch invites", err)
		return
	}
	data := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		data = append(data, gin.H{
			"id": row.ID, "first_name": row.FirstName, "last_name": row.LastName,
			"email": row.Email, "role": row.Role, "org_role": row.OrgRole,
			"created_at": row.CreatedAt, "token_expires_at": row.TokenExpiresAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (s *Server) handlerCreatePracticeInvite(c *gin.Context) {
	setPrivateNoStore(c)
	caller := c.MustGet("user").(AuthUser)
	input, err := decodeCreatePracticeInvite(c.Request.Body)
	if err != nil {
		respondTeamError(c, http.StatusBadRequest, "Invalid request data", nil)
		return
	}
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	defer tx.Rollback(c)
	if _, err := tx.Exec(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, caller.PracticeId.String()+":seats"); err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	queries := s.DBQuery.WithTx(tx)
	if _, err := queries.FindPracticeUserByEmail(c, db.FindPracticeUserByEmailParams{PracticeID: caller.PracticeId, Email: input.Email}); err == nil {
		respondTeamError(c, http.StatusConflict, "A user with this email already belongs to the practice", nil)
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	usage, err := practiceSeatUsageWithQueries(c, queries, caller.PracticeId)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	if usage.AtLimit {
		respondSeatLimit(c, usage)
		return
	}
	token := uuid.NewString()
	expiresAt := time.Now().Add(inviteTokenLifetime)
	invite, err := queries.UpsertPracticeInvite(c, db.UpsertPracticeInviteParams{
		PracticeID: *caller.PracticeId, Email: input.Email, FirstName: input.FirstName,
		LastName: input.LastName, OrgRole: input.OrgRole, Role: input.Role,
		InvitedBy: caller.ID, Token: token, TokenExpiresAt: expiresAt,
	})
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	practiceName, err := queries.GetPracticeNameForInvite(c, *caller.PracticeId)
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to create invite", err)
		return
	}
	s.sendTeamInviteBestEffort(c, TeamInviteNotification{
		Email: input.Email, FirstName: input.FirstName, PracticeName: practiceName,
		InviterName: caller.Name, Role: input.Role, Link: s.inviteLink(invite.Token),
	})
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

func (s *Server) handlerDeletePracticeInvite(c *gin.Context) {
	setPrivateNoStore(c)
	caller := c.MustGet("user").(AuthUser)
	id, ok := parseInviteID(c)
	if !ok {
		return
	}
	rows, err := s.DBQuery.DeletePracticeInvite(c, db.DeletePracticeInviteParams{ID: id, PracticeID: *caller.PracticeId})
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to delete invite", err)
		return
	}
	if rows != 1 {
		respondTeamError(c, http.StatusNotFound, "Invite not found", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerResendPracticeInvite(c *gin.Context) {
	setPrivateNoStore(c)
	caller := c.MustGet("user").(AuthUser)
	id, ok := parseInviteID(c)
	if !ok {
		return
	}
	invite, err := s.DBQuery.GetPracticeInviteForResend(c, db.GetPracticeInviteForResendParams{ID: id, PracticeID: *caller.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondTeamError(c, http.StatusNotFound, "Invite not found", nil)
		return
	}
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to resend invite", err)
		return
	}
	token := uuid.NewString()
	expiresAt := time.Now().Add(inviteTokenLifetime)
	rows, err := s.DBQuery.RotatePracticeInvite(c, db.RotatePracticeInviteParams{
		Token: token, TokenExpiresAt: expiresAt, ID: id, PracticeID: *caller.PracticeId,
	})
	if err != nil || rows != 1 {
		respondTeamError(c, http.StatusInternalServerError, "Failed to resend invite", err)
		return
	}
	s.sendTeamInviteBestEffort(c, TeamInviteNotification{
		Email: invite.Email, FirstName: invite.FirstName, PracticeName: invite.PracticeName,
		InviterName: caller.Name, Role: invite.Role, Link: s.inviteLink(token),
	})
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlerValidateInvite(c *gin.Context) {
	setPrivateNoStore(c)
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "Token is required"})
		return
	}
	if _, err := uuid.Parse(token); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"valid": false, "error": "Invite not found"})
		return
	}
	invite, err := s.DBQuery.GetInviteValidation(c, token)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"valid": false, "error": "Invite not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"valid": false, "error": "Failed to validate invite"})
		return
	}
	if invite.AcceptedAt != nil {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "Invite already accepted"})
		return
	}
	if invite.TokenExpiresAt.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"valid": false, "error": "Invite expired"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "invite": gin.H{
		"id": invite.ID, "first_name": invite.FirstName, "last_name": invite.LastName,
		"practice_name": invite.PracticeName, "role": invite.Role,
		"org_role": invite.OrgRole, "email": invite.Email,
	}})
}

func decodeCreatePracticeInvite(body io.Reader) (createPracticeInviteInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return createPracticeInviteInput{}, errors.New("invalid request")
	}
	first, err := requiredInviteString(fields, "first_name", 100)
	if err != nil {
		return createPracticeInviteInput{}, err
	}
	last, err := requiredInviteString(fields, "last_name", 100)
	if err != nil {
		return createPracticeInviteInput{}, err
	}
	email, err := requiredInviteString(fields, "email", 320)
	if err != nil {
		return createPracticeInviteInput{}, err
	}
	email = strings.ToLower(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return createPracticeInviteInput{}, errors.New("invalid email")
	}
	role, err := requiredInviteString(fields, "role", 20)
	if err != nil || (role != "admin" && role != "staff") {
		return createPracticeInviteInput{}, errors.New("invalid role")
	}
	return createPracticeInviteInput{
		FirstName: first, LastName: last, Email: email, Role: role,
		OrgRole: optionalInviteString(fields, "org_role", 100),
	}, nil
}

func requiredInviteString(fields map[string]json.RawMessage, name string, max int) (string, error) {
	raw, ok := fields[name]
	if !ok {
		return "", errors.New("missing field")
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", errors.New("invalid field")
	}
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > max {
		return "", errors.New("invalid field")
	}
	return value, nil
}

func optionalInviteString(fields map[string]json.RawMessage, name string, max int) *string {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > max {
		return nil
	}
	return &value
}

func (s *Server) sendTeamInviteBestEffort(c *gin.Context, notification TeamInviteNotification) {
	if s.teamInviteNotify != nil {
		_, _ = s.teamInviteNotify.SendTeamInvite(c, notification)
	}
}

func (s *Server) inviteLink(token string) string {
	return s.inviteBaseURL + "/invite?token=" + token
}

func parseInviteID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondTeamError(c, http.StatusBadRequest, "Invalid invite ID", nil)
		return uuid.Nil, false
	}
	return id, true
}

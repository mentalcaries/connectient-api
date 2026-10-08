package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type acceptInviteInput struct {
	Token       string
	FirstName   string
	LastName    string
	MobilePhone *string
	TermsAgreed bool
}

func (s *Server) handlerAcceptInvite(c *gin.Context) {
	setPrivateNoStore(c)
	claims := c.MustGet("claims").(TokenClaims)
	input, err := decodeAcceptInvite(c.Request.Body)
	if err != nil {
		respondInviteAcceptanceError(c, http.StatusBadRequest, err.Error(), "")
		return
	}
	if claims.Email == "" {
		respondInviteAcceptanceError(c, http.StatusUnauthorized, "Authentication required", "")
		return
	}
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to accept invite", "")
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	targetPracticeID, err := queries.GetInviteAcceptanceTarget(c, input.Token)
	if errors.Is(err, pgx.ErrNoRows) {
		respondInviteAcceptanceError(c, http.StatusBadRequest, "Invalid or expired invite", "")
		return
	}
	if err != nil {
		respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to accept invite", "")
		return
	}
	if _, err := tx.Exec(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, targetPracticeID.String()+":seats"); err != nil {
		respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to accept invite", "")
		return
	}
	invite, err := queries.LockInviteAcceptance(c, input.Token)
	if errors.Is(err, pgx.ErrNoRows) {
		respondInviteAcceptanceError(c, http.StatusBadRequest, "Invalid or expired invite", "")
		return
	}
	if err != nil {
		respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to accept invite", "")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(claims.Email), strings.TrimSpace(invite.Email)) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "email_mismatch",
			"message": "This invite was sent to a different email address. Please sign in with the correct account.",
		})
		return
	}
	membership, membershipErr := queries.GetIdentityMembershipForUpdate(c, claims.ID)
	displayName := input.FirstName + " " + input.LastName
	if membershipErr == nil {
		if membership.PracticeID == nil || *membership.PracticeID != invite.PracticeID || membership.DeletedAt != nil || !membership.IsActive ||
			membership.Email == nil || !strings.EqualFold(*membership.Email, invite.Email) || invite.AcceptedAt == nil {
			respondInviteAcceptanceError(c, http.StatusConflict, "Identity already belongs to another membership", "membership_conflict")
			return
		}
		displayName = membership.FirstName + " " + membership.LastName
	} else if !errors.Is(membershipErr, pgx.ErrNoRows) {
		respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to accept invite", "")
		return
	} else {
		if invite.AcceptedAt != nil {
			respondInviteAcceptanceError(c, http.StatusBadRequest, "Invite already accepted", "")
			return
		}
		if invite.TokenExpiresAt.Before(time.Now()) {
			respondInviteAcceptanceError(c, http.StatusBadRequest, "Invite expired", "")
			return
		}
		if err := queries.CreateInvitedMembership(c, db.CreateInvitedMembershipParams{
			ID: claims.ID, PracticeID: &invite.PracticeID, Email: &claims.Email,
			FirstName: input.FirstName, LastName: input.LastName, MobilePhone: input.MobilePhone,
			OrgRole: invite.OrgRole, Role: &invite.Role, InvitedBy: &invite.InvitedBy,
		}); err != nil {
			respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to create user account", "")
			return
		}
	}
	if invite.AcceptedAt == nil {
		rows, err := queries.MarkInviteAccepted(c, invite.ID)
		if err != nil || rows != 1 {
			respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to complete invite acceptance", "")
			return
		}
	}
	if err := tx.Commit(c); err != nil {
		respondInviteAcceptanceError(c, http.StatusInternalServerError, "Failed to complete invite acceptance", "")
		return
	}
	identityErr := ErrIdentityProfileUnavailable
	if s.identityProfiles != nil {
		identityErr = s.identityProfiles.UpdateDisplayName(c, claims.ID, displayName)
	}
	if identityErr != nil {
		log.Printf("invite acceptance identity sync failed for user %s: %v", claims.ID, identityErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false, "error": "identity_sync_failed", "membership_created": true,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodeAcceptInvite(body io.Reader) (acceptInviteInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return acceptInviteInput{}, errors.New("Invalid request data")
	}
	token, err := requiredInviteString(fields, "token", 36)
	if err != nil {
		return acceptInviteInput{}, errors.New("Invalid request data")
	}
	if _, err := uuid.Parse(token); err != nil {
		return acceptInviteInput{}, errors.New("Invalid request data")
	}
	first, err := requiredInviteString(fields, "first_name", 100)
	if err != nil {
		return acceptInviteInput{}, errors.New("Invalid request data")
	}
	last, err := requiredInviteString(fields, "last_name", 100)
	if err != nil {
		return acceptInviteInput{}, errors.New("Invalid request data")
	}
	var terms bool
	if raw, ok := fields["termsAgreed"]; !ok || json.Unmarshal(raw, &terms) != nil {
		return acceptInviteInput{}, errors.New("Invalid request data")
	}
	if !terms {
		return acceptInviteInput{}, errors.New("terms_required")
	}
	var mobile *string
	if raw, ok := fields["mobile_phone"]; ok && !isJSONNull(raw) {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return acceptInviteInput{}, errors.New("Invalid request data")
		}
		value = strings.TrimSpace(value)
		if utf8.RuneCountInString(value) > 20 {
			return acceptInviteInput{}, errors.New("Invalid request data")
		}
		if value != "" {
			mobile = &value
		}
	}
	return acceptInviteInput{
		Token: token, FirstName: first, LastName: last,
		MobilePhone: mobile, TermsAgreed: terms,
	}, nil
}

func respondInviteAcceptanceError(c *gin.Context, status int, message, code string) {
	body := gin.H{"success": false, "error": message}
	if code != "" {
		body["error"] = code
		body["message"] = message
	}
	c.JSON(status, body)
}

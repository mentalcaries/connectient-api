package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func (s *Server) handlerSendRegistrationEmail(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, ok := parseRegistrationID(c)
	if !ok {
		return
	}
	registration, ok := s.registrationForDelivery(c, id, *user.PracticeId, "Registration is already completed")
	if !ok || !registrationLinkUsable(c, registration) {
		return
	}
	override := decodeOptionalRegistrationContact(c.Request.Body, "email")
	target := registration.PatientEmail
	if override != nil {
		target = normalizedValidRegistrationEmail(override)
	}
	if target == nil {
		respondRegistrationError(c, http.StatusBadRequest, "Enter a valid patient email", nil)
		return
	}
	rows, err := s.DBQuery.UpdateRegistrationEmail(c, db.UpdateRegistrationEmailParams{
		PatientEmail: target, ID: id, PracticeID: *user.PracticeId,
	})
	if err != nil || rows != 1 {
		if isUniqueViolation(err) {
			respondRegistrationError(c, http.StatusConflict, "A registration already exists for this patient at this practice.", nil)
			return
		}
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to save patient email", err)
		return
	}
	result, deliveryErr := s.sendRegistrationEmail(c, RegistrationNotification{
		PatientName: registration.PatientName, PatientEmail: target,
		PatientPhone: registration.PatientPhone, Link: s.registrationLink(registration.Token),
	})
	if deliveryErr != nil || !registrationDeliverySucceeded(result) {
		restored := s.restoreRegistrationEmail(c, id, *user.PracticeId, registration.PatientEmail, target)
		if !restored {
			respondRegistrationError(c, http.StatusInternalServerError, "Delivery failed and the previous email could not be restored", deliveryErr)
			return
		}
		respondRegistrationDeliveryFailure(c, "Email", result, deliveryErr)
		return
	}
	trackingFailed := s.markRegistrationSent(c, id, *user.PracticeId)
	c.JSON(http.StatusOK, gin.H{
		"message": "Registration link sent successfully", "email": *target,
		"delivery_tracking_failed": trackingFailed,
	})
}

func (s *Server) handlerSendRegistrationWhatsApp(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, ok := parseRegistrationID(c)
	if !ok {
		return
	}
	registration, ok := s.registrationForDelivery(c, id, *user.PracticeId, "Registration is already completed")
	if !ok || !registrationLinkUsable(c, registration) {
		return
	}
	override := decodeOptionalRegistrationContact(c.Request.Body, "phone")
	target := registration.PatientPhone
	if override != nil {
		target = normalizedValidRegistrationPhone(override)
	}
	if target == nil && registration.AppointmentID != nil {
		appointment, err := s.DBQuery.GetRegistrationAppointment(c, db.GetRegistrationAppointmentParams{
			ID: *registration.AppointmentID, PracticeID: *user.PracticeId,
		})
		if err == nil {
			target = normalizedValidRegistrationPhone(&appointment.MobilePhone)
		}
	}
	if target == nil {
		respondRegistrationError(c, http.StatusBadRequest, "Enter a valid WhatsApp number", nil)
		return
	}
	rows, err := s.DBQuery.UpdateRegistrationPhone(c, db.UpdateRegistrationPhoneParams{
		PatientPhone: target, ID: id, PracticeID: *user.PracticeId,
	})
	if err != nil || rows != 1 {
		if isUniqueViolation(err) {
			respondRegistrationError(c, http.StatusConflict, "A registration already exists for this patient at this practice.", nil)
			return
		}
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to save WhatsApp number", err)
		return
	}
	result, deliveryErr := s.sendRegistrationWhatsApp(c, RegistrationNotification{
		PatientName: registration.PatientName, PatientEmail: registration.PatientEmail,
		PatientPhone: target, Link: s.registrationLink(registration.Token),
	})
	if deliveryErr != nil || !registrationDeliverySucceeded(result) {
		restored := s.restoreRegistrationPhone(c, id, *user.PracticeId, registration.PatientPhone, target)
		if !restored {
			respondRegistrationError(c, http.StatusInternalServerError, "Delivery failed and the previous phone could not be restored", deliveryErr)
			return
		}
		respondRegistrationDeliveryFailure(c, "WhatsApp", result, deliveryErr)
		return
	}
	trackingFailed := s.markRegistrationSent(c, id, *user.PracticeId)
	c.JSON(http.StatusOK, gin.H{
		"message":                  "Registration link sent via WhatsApp.",
		"delivery_tracking_failed": trackingFailed,
	})
}

func (s *Server) handlerResendRegistration(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, ok := parseRegistrationID(c)
	if !ok {
		return
	}
	registration, ok := s.registrationForDelivery(c, id, *user.PracticeId, "Cannot resend a completed registration")
	if !ok {
		return
	}
	if registration.PatientEmail == nil {
		respondRegistrationError(c, http.StatusBadRequest, "No email found for this registration", nil)
		return
	}
	newToken, err := newRegistrationToken()
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to regenerate token", err)
		return
	}
	expiresAt := time.Now().Add(registrationTokenLifetime)
	updated, err := s.DBQuery.RotateRegistrationToken(c, db.RotateRegistrationTokenParams{
		Token: newToken, TokenExpiresAt: expiresAt, ID: id, PracticeID: *user.PracticeId,
	})
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to regenerate token", err)
		return
	}
	result, deliveryErr := s.sendRegistrationEmail(c, RegistrationNotification{
		PatientName: registration.PatientName, PatientEmail: registration.PatientEmail,
		PatientPhone: registration.PatientPhone, Link: s.registrationLink(newToken),
	})
	if deliveryErr != nil || !registrationDeliverySucceeded(result) {
		rows, restoreErr := s.DBQuery.RestoreRegistrationToken(c, db.RestoreRegistrationTokenParams{
			PreviousToken: registration.Token, PreviousTokenExpiresAt: registration.TokenExpiresAt,
			PreviousStatus: registration.Status, PreviousSentAt: registration.SentAt,
			ID: id, PracticeID: *user.PracticeId, ExpectedToken: newToken,
		})
		if restoreErr != nil || rows != 1 {
			respondRegistrationError(c, http.StatusInternalServerError, "Delivery failed and the previous token could not be restored", restoreErr)
			return
		}
		respondRegistrationDeliveryFailure(c, "Email", result, deliveryErr)
		return
	}
	trackingFailed := s.markRegistrationSent(c, id, *user.PracticeId)
	c.JSON(http.StatusOK, gin.H{
		"id": id, "token": updated.Token, "link": s.registrationLink(updated.Token),
		"expires_at": updated.TokenExpiresAt, "status": updated.Status,
		"delivery_tracking_failed": trackingFailed,
	})
}

func (s *Server) registrationForDelivery(c *gin.Context, id, practiceID uuid.UUID, completedMessage string) (db.GetRegistrationForLinkRow, bool) {
	row, err := s.DBQuery.GetRegistrationForLink(c, db.GetRegistrationForLinkParams{ID: id, PracticeID: practiceID})
	if errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusNotFound, "Registration not found", nil)
		return db.GetRegistrationForLinkRow{}, false
	}
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to fetch registration", err)
		return db.GetRegistrationForLinkRow{}, false
	}
	if row.Status == "completed" {
		respondRegistrationError(c, http.StatusBadRequest, completedMessage, nil)
		return db.GetRegistrationForLinkRow{}, false
	}
	return row, true
}

func registrationLinkUsable(c *gin.Context, row db.GetRegistrationForLinkRow) bool {
	if row.Status == "expired" || row.TokenExpiresAt.Before(time.Now()) {
		respondRegistrationError(c, http.StatusGone, "Registration link has expired; resend to generate a new link", nil)
		return false
	}
	return true
}

func decodeOptionalRegistrationContact(body io.Reader, field string) *string {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if decoder.Decode(&fields) != nil || fields == nil {
		return nil
	}
	value, ok := registrationString(fields[field])
	if !ok {
		return nil
	}
	return &value
}

func (s *Server) sendRegistrationEmail(c *gin.Context, notification RegistrationNotification) (RegistrationDeliveryResult, error) {
	if s.registrationNotify == nil {
		return RegistrationDeliveryUnavailable, nil
	}
	return s.registrationNotify.SendRegistrationEmail(c, notification)
}

func (s *Server) sendRegistrationWhatsApp(c *gin.Context, notification RegistrationNotification) (RegistrationDeliveryResult, error) {
	if s.registrationNotify == nil {
		return RegistrationDeliveryUnavailable, nil
	}
	return s.registrationNotify.SendRegistrationWhatsApp(c, notification)
}

func (s *Server) restoreRegistrationEmail(c *gin.Context, id, practiceID uuid.UUID, previous, expected *string) bool {
	rows, err := s.DBQuery.RestoreRegistrationEmail(c, db.RestoreRegistrationEmailParams{
		PreviousEmail: previous, ID: id, PracticeID: practiceID, ExpectedEmail: expected,
	})
	return err == nil && rows == 1
}

func (s *Server) restoreRegistrationPhone(c *gin.Context, id, practiceID uuid.UUID, previous, expected *string) bool {
	rows, err := s.DBQuery.RestoreRegistrationPhone(c, db.RestoreRegistrationPhoneParams{
		PreviousPhone: previous, ID: id, PracticeID: practiceID, ExpectedPhone: expected,
	})
	return err == nil && rows == 1
}

func (s *Server) markRegistrationSent(c *gin.Context, id, practiceID uuid.UUID) bool {
	rows, err := s.DBQuery.MarkRegistrationSent(c, db.MarkRegistrationSentParams{ID: id, PracticeID: practiceID})
	return err != nil || rows != 1
}

func respondRegistrationDeliveryFailure(c *gin.Context, channel string, result RegistrationDeliveryResult, err error) {
	if result == RegistrationDeliveryUnavailable && err == nil {
		respondRegistrationError(c, http.StatusServiceUnavailable, channel+" delivery is not configured", nil)
		return
	}
	message := "Failed to send " + channel
	if channel == "WhatsApp" {
		message += " message"
	}
	respondRegistrationError(c, http.StatusInternalServerError, message, err)
}

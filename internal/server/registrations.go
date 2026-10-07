package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const registrationTokenLifetime = 7 * 24 * time.Hour

var registrationPhonePattern = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

type RegistrationNotification struct {
	PatientName  string
	PatientEmail *string
	PatientPhone *string
	Link         string
}

type RegistrationNotifier interface {
	SendRegistrationEmail(context.Context, RegistrationNotification) (RegistrationDeliveryResult, error)
	SendRegistrationWhatsApp(context.Context, RegistrationNotification) (RegistrationDeliveryResult, error)
}

type RegistrationDeliveryResult string

const (
	RegistrationDeliverySent        RegistrationDeliveryResult = "sent"
	RegistrationDeliverySimulated   RegistrationDeliveryResult = "simulated"
	RegistrationDeliveryUnavailable RegistrationDeliveryResult = "unavailable"
	RegistrationDeliveryFailed      RegistrationDeliveryResult = "failed"
)

type createRegistrationInput struct {
	AppointmentID *uuid.UUID
	PatientName   string
	PatientEmail  *string
	PatientPhone  *string
	SendEmail     bool
	SendWhatsApp  bool
}

type RegistrationListResponse struct {
	ID            uuid.UUID  `json:"id"`
	PatientName   string     `json:"patient_name"`
	PatientEmail  *string    `json:"patient_email"`
	PatientPhone  *string    `json:"patient_phone"`
	Status        string     `json:"status"`
	SentAt        *time.Time `json:"sent_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	AppointmentID *uuid.UUID `json:"appointment_id"`
	CreatedAt     time.Time  `json:"created_at"`
	Token         string     `json:"token"`
}

func (s *Server) handlerListRegistrations(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	search := strings.TrimSpace(c.Query("search"))
	var appointmentID *uuid.UUID
	if raw := strings.TrimSpace(c.Query("appointment_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			respondRegistrationError(c, http.StatusBadRequest, "Invalid appointment ID", nil)
			return
		}
		appointmentID = &parsed
	}
	rows, err := s.DBQuery.ListRegistrations(c, db.ListRegistrationsParams{
		PracticeID: *user.PracticeId, Search: search, AppointmentID: appointmentID,
	})
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to fetch registrations", err)
		return
	}
	registrations := make([]RegistrationListResponse, 0, len(rows))
	for _, row := range rows {
		registrations = append(registrations, RegistrationListResponse{
			ID: row.ID, PatientName: row.PatientName, PatientEmail: row.PatientEmail,
			PatientPhone: row.PatientPhone, Status: row.Status, SentAt: row.SentAt,
			CompletedAt: row.CompletedAt, AppointmentID: row.AppointmentID,
			CreatedAt: row.CreatedAt, Token: row.Token,
		})
	}
	c.JSON(http.StatusOK, registrations)
}

func (s *Server) handlerGetRegistration(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, ok := parseRegistrationID(c)
	if !ok {
		return
	}
	row, err := s.DBQuery.GetRegistrationDetail(c, db.GetRegistrationDetailParams{ID: id, PracticeID: *user.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusNotFound, "Registration not found", nil)
		return
	}
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to fetch registration", err)
		return
	}
	var formData json.RawMessage
	if len(row.FormData) > 0 {
		formData = row.FormData
	}
	c.JSON(http.StatusOK, gin.H{
		"id": row.ID, "patient_name": row.PatientName, "patient_email": row.PatientEmail,
		"patient_phone": row.PatientPhone, "status": row.Status, "sent_at": row.SentAt,
		"completed_at": row.CompletedAt, "appointment_id": row.AppointmentID,
		"created_at": row.CreatedAt, "form_data": formData, "form_version": row.FormVersion,
	})
}

func (s *Server) handlerCreateRegistration(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	input, err := decodeCreateRegistration(c.Request.Body)
	if err != nil {
		respondRegistrationError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if input.AppointmentID != nil {
		appointment, err := s.DBQuery.GetRegistrationAppointment(c, db.GetRegistrationAppointmentParams{
			ID: *input.AppointmentID, PracticeID: *user.PracticeId,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			respondRegistrationError(c, http.StatusNotFound, "Appointment not found", nil)
			return
		}
		if err != nil {
			respondRegistrationError(c, http.StatusInternalServerError, "Failed to create registration", err)
			return
		}
		if input.PatientEmail == nil {
			input.PatientEmail = normalizedValidRegistrationEmail(&appointment.Email)
		}
		if input.PatientPhone == nil {
			input.PatientPhone = normalizedValidRegistrationPhone(&appointment.MobilePhone)
		}
	}
	if err := validateRegistrationContacts(input); err != nil {
		respondRegistrationError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	_, err = s.DBQuery.GetRegistrationDuplicate(c, db.GetRegistrationDuplicateParams{
		PracticeID: *user.PracticeId, PatientName: input.PatientName,
		PatientEmail: input.PatientEmail, PatientPhone: input.PatientPhone,
	})
	if err == nil {
		respondRegistrationError(c, http.StatusConflict, "A registration already exists for this patient at this practice.", nil)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to create registration", err)
		return
	}
	token, err := newRegistrationToken()
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to create registration", err)
		return
	}
	expiresAt := time.Now().Add(registrationTokenLifetime)
	registration, err := s.DBQuery.CreatePatientRegistration(c, db.CreatePatientRegistrationParams{
		PracticeID: *user.PracticeId, AppointmentID: input.AppointmentID,
		PatientName: input.PatientName, PatientEmail: input.PatientEmail,
		PatientPhone: input.PatientPhone, Token: token, TokenExpiresAt: expiresAt,
		SentByUserID: user.ID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			respondRegistrationError(c, http.StatusConflict, "A registration already exists for this patient at this practice.", nil)
			return
		}
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to create registration", err)
		return
	}
	link := s.registrationLink(token)
	var notificationSent *bool
	deliveryTrackingFailed := false
	if input.SendEmail || input.SendWhatsApp {
		sent := false
		notificationSent = &sent
		if s.registrationNotify != nil {
			notification := RegistrationNotification{
				PatientName: input.PatientName, PatientEmail: input.PatientEmail,
				PatientPhone: input.PatientPhone, Link: link,
			}
			if input.SendEmail {
				var result RegistrationDeliveryResult
				result, err = s.registrationNotify.SendRegistrationEmail(c, notification)
				sent = err == nil && registrationDeliverySucceeded(result)
			} else {
				var result RegistrationDeliveryResult
				result, err = s.registrationNotify.SendRegistrationWhatsApp(c, notification)
				sent = err == nil && registrationDeliverySucceeded(result)
			}
			if sent {
				notificationSent = &sent
				rows, trackingErr := s.DBQuery.MarkRegistrationSent(c, db.MarkRegistrationSentParams{
					ID: registration.ID, PracticeID: *user.PracticeId,
				})
				deliveryTrackingFailed = trackingErr != nil || rows != 1
			}
		}
	}
	c.JSON(http.StatusCreated, gin.H{
		"id": registration.ID, "token": registration.Token, "link": link,
		"expires_at": registration.TokenExpiresAt, "status": registration.Status,
		"notification_sent": notificationSent, "delivery_tracking_failed": deliveryTrackingFailed,
	})
}

func (s *Server) handlerGetRegistrationLink(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, ok := parseRegistrationID(c)
	if !ok {
		return
	}
	row, err := s.DBQuery.GetRegistrationForLink(c, db.GetRegistrationForLinkParams{ID: id, PracticeID: *user.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusNotFound, "Registration not found", nil)
		return
	}
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to fetch registration link", err)
		return
	}
	if row.Status == "completed" {
		respondRegistrationError(c, http.StatusBadRequest, "Registration is already completed", nil)
		return
	}
	token, expiresAt, wasRegenerated := row.Token, row.TokenExpiresAt, false
	if row.Status == "expired" || expiresAt.Before(time.Now()) {
		token, err = newRegistrationToken()
		if err != nil {
			respondRegistrationError(c, http.StatusInternalServerError, "Failed to regenerate token", err)
			return
		}
		expiresAt = time.Now().Add(registrationTokenLifetime)
		updated, err := s.DBQuery.RotateRegistrationToken(c, db.RotateRegistrationTokenParams{
			Token: token, TokenExpiresAt: expiresAt, ID: id, PracticeID: *user.PracticeId,
		})
		if err != nil {
			respondRegistrationError(c, http.StatusInternalServerError, "Failed to regenerate token", err)
			return
		}
		token, expiresAt, wasRegenerated = updated.Token, updated.TokenExpiresAt, true
	}
	c.JSON(http.StatusOK, gin.H{
		"id": row.ID, "patient_name": row.PatientName, "patient_email": row.PatientEmail,
		"patient_phone": row.PatientPhone, "link": s.registrationLink(token),
		"expires_at": expiresAt, "was_regenerated": wasRegenerated,
	})
}

func (s *Server) handlerDeleteRegistration(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, ok := parseRegistrationID(c)
	if !ok {
		return
	}
	state, err := s.DBQuery.GetRegistrationDeleteState(c, db.GetRegistrationDeleteStateParams{ID: id, PracticeID: *user.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusNotFound, "Registration not found", nil)
		return
	}
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to delete registration", err)
		return
	}
	if state == "completed" {
		respondRegistrationError(c, http.StatusBadRequest, "Completed registrations cannot be deleted", nil)
		return
	}
	if _, err := s.DBQuery.SoftDeleteRegistration(c, db.SoftDeleteRegistrationParams{ID: id, PracticeID: *user.PracticeId}); err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to delete registration", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Registration deleted."})
}

func decodeCreateRegistration(body io.Reader) (createRegistrationInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return createRegistrationInput{}, errors.New("Invalid request body")
	}
	var input createRegistrationInput
	name, ok := registrationString(fields["patient_name"])
	if !ok {
		return input, errors.New("patient_name is required")
	}
	input.PatientName = name
	if raw, exists := fields["appointment_id"]; exists && !isJSONNull(raw) {
		value, ok := registrationString(raw)
		if !ok {
			return input, errors.New("Invalid appointment ID")
		}
		id, err := uuid.Parse(value)
		if err != nil {
			return input, errors.New("Invalid appointment ID")
		}
		input.AppointmentID = &id
	}
	if value, ok := registrationString(fields["patient_email"]); ok {
		input.PatientEmail = normalizedValidRegistrationEmail(&value)
	}
	if value, ok := registrationString(fields["patient_phone"]); ok {
		input.PatientPhone = normalizedValidRegistrationPhone(&value)
	}
	var err error
	if raw, exists := fields["send_email"]; exists {
		input.SendEmail, err = decodeBoolean(raw, "send_email")
		if err != nil {
			return input, err
		}
	}
	if raw, exists := fields["send_whatsapp"]; exists {
		input.SendWhatsApp, err = decodeBoolean(raw, "send_whatsapp")
		if err != nil {
			return input, err
		}
	}
	if input.SendEmail && input.SendWhatsApp {
		return input, errors.New("Choose either email or WhatsApp, not both")
	}
	return input, nil
}

func validateRegistrationContacts(input createRegistrationInput) error {
	if input.PatientEmail == nil && input.PatientPhone == nil {
		return errors.New("A patient email or WhatsApp number is required")
	}
	if input.SendEmail && input.PatientEmail == nil {
		return errors.New("A patient email is required to send via email")
	}
	if input.SendWhatsApp && input.PatientPhone == nil {
		return errors.New("A WhatsApp number is required to send via WhatsApp")
	}
	return nil
}

func registrationString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || isJSONNull(raw) {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}

func normalizedValidRegistrationEmail(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	address, err := mail.ParseAddress(normalized)
	if err != nil || address.Address != normalized {
		return nil
	}
	return &normalized
}

func normalizedValidRegistrationPhone(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if !registrationPhonePattern.MatchString(normalized) {
		return nil
	}
	return &normalized
}

func newRegistrationToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (s *Server) registrationLink(token string) string {
	return s.patientBaseURL + "/register/" + token
}

func registrationDeliverySucceeded(result RegistrationDeliveryResult) bool {
	return result == RegistrationDeliverySent || result == RegistrationDeliverySimulated
}

func parseRegistrationID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondRegistrationError(c, http.StatusBadRequest, "Invalid registration ID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func respondRegistrationError(c *gin.Context, status int, message string, err error) {
	respondPatientError(c, status, message, err)
}

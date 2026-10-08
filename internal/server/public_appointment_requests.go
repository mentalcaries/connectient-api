package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const publicAppointmentRequestBodyLimit = 16 << 10

var publicIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

type PublicAppointmentRequestNotification struct {
	AppointmentID   uuid.UUID
	PracticeID      uuid.UUID
	PracticeName    string
	PracticeEmail   *string
	FirstName       string
	LastName        string
	Email           string
	MobilePhone     string
	RequestedDate   string
	RequestedTime   string
	AppointmentType string
}

type PublicAppointmentRequestNotifier interface {
	NotifyStaffAppointmentRequest(context.Context, PublicAppointmentRequestNotification) error
}

type publicAppointmentRequestInput struct {
	FirstName           string     `json:"first_name"`
	LastName            string     `json:"last_name"`
	Email               string     `json:"email"`
	MobilePhone         string     `json:"mobile_phone"`
	RequestedDate       string     `json:"requested_date"`
	RequestedTime       string     `json:"requested_time"`
	AppointmentType     string     `json:"appointment_type"`
	Description         *string    `json:"description"`
	IsEmergency         bool       `json:"is_emergency"`
	PreferredProviderID *uuid.UUID `json:"preferred_provider_id"`
	LocationID          *uuid.UUID `json:"location_id"`
}

type publicAppointmentRequestResponse struct {
	ID     uuid.UUID `json:"id"`
	Status string    `json:"status"`
}

func (s *Server) handlerCreatePublicAppointmentRequest(c *gin.Context) {
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if !publicIdempotencyKeyPattern.MatchString(idempotencyKey) {
		respondPublicBookingError(c, http.StatusBadRequest, "A valid Idempotency-Key header is required", nil)
		return
	}
	practice, ok := s.publicBookingPractice(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, publicAppointmentRequestBodyLimit)
	input, err := decodePublicAppointmentRequest(c.Request.Body)
	if err != nil {
		respondPublicBookingError(c, http.StatusBadRequest, "Invalid appointment request", nil)
		return
	}
	response, replayed, err := s.createPublicAppointmentRequest(c, practice, idempotencyKey, input)
	if err != nil {
		var requestError *schedulingError
		if errors.As(err, &requestError) {
			respondPublicBookingError(c, requestError.Status, requestError.Message, nil)
			return
		}
		respondPublicBookingError(c, http.StatusInternalServerError, "Failed to create appointment request", err)
		return
	}
	if replayed {
		c.Header("Idempotent-Replayed", "true")
		c.Data(http.StatusCreated, "application/json; charset=utf-8", response)
		return
	}
	var payload publicAppointmentRequestResponse
	if err := json.Unmarshal(response, &payload); err != nil {
		respondPublicBookingError(c, http.StatusInternalServerError, "Failed to create appointment request", err)
		return
	}
	s.broadcastAppointmentChange(c, practice.ID, "INSERT", payload.ID)
	if s.publicBookingNotify != nil {
		notification := PublicAppointmentRequestNotification{
			AppointmentID: payload.ID, PracticeID: practice.ID,
			PracticeName: practice.Name, PracticeEmail: practice.Email,
			FirstName: input.FirstName, LastName: input.LastName, Email: input.Email,
			MobilePhone: input.MobilePhone, RequestedDate: input.RequestedDate,
			RequestedTime: input.RequestedTime, AppointmentType: input.AppointmentType,
		}
		if err := s.publicBookingNotify.NotifyStaffAppointmentRequest(c, notification); err != nil {
			log.Printf("public appointment staff notification failed for %s: %v", payload.ID, err)
		}
	}
	c.Data(http.StatusCreated, "application/json; charset=utf-8", response)
}

func decodePublicAppointmentRequest(body io.Reader) (publicAppointmentRequestInput, error) {
	var wire struct {
		FirstName           string  `json:"first_name"`
		LastName            string  `json:"last_name"`
		Email               string  `json:"email"`
		MobilePhone         string  `json:"mobile_phone"`
		RequestedDate       string  `json:"requested_date"`
		RequestedTime       string  `json:"requested_time"`
		AppointmentType     string  `json:"appointment_type"`
		Description         *string `json:"description"`
		IsEmergency         bool    `json:"is_emergency"`
		PreferredProviderID *string `json:"preferred_provider_id"`
		LocationID          *string `json:"location_id"`
	}
	if err := decodeStrictJSON(body, &wire); err != nil {
		return publicAppointmentRequestInput{}, err
	}
	wire.FirstName, wire.LastName = strings.TrimSpace(wire.FirstName), strings.TrimSpace(wire.LastName)
	if utf8.RuneCountInString(wire.FirstName) < 2 || utf8.RuneCountInString(wire.FirstName) > 100 ||
		utf8.RuneCountInString(wire.LastName) < 2 || utf8.RuneCountInString(wire.LastName) > 100 {
		return publicAppointmentRequestInput{}, errors.New("invalid patient name")
	}
	wire.Email = strings.ToLower(strings.TrimSpace(wire.Email))
	address, err := mail.ParseAddress(wire.Email)
	if err != nil || address.Address != wire.Email || len(wire.Email) > 254 {
		return publicAppointmentRequestInput{}, errors.New("invalid email")
	}
	wire.MobilePhone = strings.TrimSpace(wire.MobilePhone)
	if digits := normalizePatientPhone(wire.MobilePhone); len(digits) < 10 || len(digits) > 15 {
		return publicAppointmentRequestInput{}, errors.New("invalid mobile phone")
	}
	date, err := time.Parse("2006-01-02", wire.RequestedDate)
	if err != nil || date.Before(time.Now().UTC().Truncate(24*time.Hour)) {
		return publicAppointmentRequestInput{}, errors.New("invalid requested date")
	}
	if !slices.Contains([]string{"morning", "afternoon", "flexible"}, wire.RequestedTime) {
		return publicAppointmentRequestInput{}, errors.New("invalid requested time")
	}
	wire.AppointmentType = strings.TrimSpace(wire.AppointmentType)
	if wire.AppointmentType == "" || utf8.RuneCountInString(wire.AppointmentType) > 150 {
		return publicAppointmentRequestInput{}, errors.New("invalid appointment type")
	}
	if wire.Description != nil {
		description := strings.TrimSpace(*wire.Description)
		if utf8.RuneCountInString(description) > 2000 {
			return publicAppointmentRequestInput{}, errors.New("description too long")
		}
		if description == "" {
			wire.Description = nil
		} else {
			wire.Description = &description
		}
	}
	providerID, err := parsePublicOptionalUUID(wire.PreferredProviderID)
	if err != nil {
		return publicAppointmentRequestInput{}, err
	}
	locationID, err := parsePublicOptionalUUID(wire.LocationID)
	if err != nil {
		return publicAppointmentRequestInput{}, err
	}
	return publicAppointmentRequestInput{
		FirstName: wire.FirstName, LastName: wire.LastName, Email: wire.Email,
		MobilePhone: wire.MobilePhone, RequestedDate: wire.RequestedDate,
		RequestedTime: wire.RequestedTime, AppointmentType: wire.AppointmentType,
		Description: wire.Description, IsEmergency: wire.IsEmergency,
		PreferredProviderID: providerID, LocationID: locationID,
	}, nil
}

func parsePublicOptionalUUID(value *string) (*uuid.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *Server) createPublicAppointmentRequest(ctx context.Context, practice db.GetPublicBookingPracticeRow, key string, input publicAppointmentRequestInput) ([]byte, bool, error) {
	canonical, err := json.Marshal(input)
	if err != nil {
		return nil, false, err
	}
	hash := sha256.Sum256(canonical)
	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	queries := s.DBQuery.WithTx(tx)
	claimed, err := queries.ClaimPublicAppointmentIdempotency(ctx, db.ClaimPublicAppointmentIdempotencyParams{
		PracticeID: practice.ID, IdempotencyKey: key, RequestHash: hash[:],
	})
	if err != nil {
		return nil, false, err
	}
	if claimed == 0 {
		existing, err := queries.LockPublicAppointmentIdempotency(ctx, db.LockPublicAppointmentIdempotencyParams{
			PracticeID: practice.ID, IdempotencyKey: key,
		})
		if err != nil {
			return nil, false, err
		}
		if !bytes.Equal(existing.RequestHash, hash[:]) {
			return nil, false, &schedulingError{Status: http.StatusConflict, Message: "Idempotency-Key was already used for a different request"}
		}
		if len(existing.ResponseBody) == 0 {
			return nil, false, errors.New("idempotency response is incomplete")
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return existing.ResponseBody, true, nil
	}
	date, _ := time.Parse("2006-01-02", input.RequestedDate)
	if err := validatePublicAppointmentResources(ctx, queries, practice, input, date); err != nil {
		return nil, false, err
	}
	patientID, err := findOrCreatePublicBookingPatient(ctx, tx, queries, practice.ID, input)
	if err != nil {
		return nil, false, err
	}
	appointmentType := input.AppointmentType
	appointment, err := queries.CreatePublicAppointmentRequest(ctx, db.CreatePublicAppointmentRequestParams{
		PracticeID: practice.ID, PatientID: patientID, FirstName: input.FirstName,
		LastName: input.LastName, Email: input.Email, MobilePhone: input.MobilePhone,
		RequestedDate: date, RequestedTime: input.RequestedTime,
		IsEmergency: input.IsEmergency, Description: input.Description,
		AppointmentType: &appointmentType, ProviderID: input.PreferredProviderID,
		LocationID: input.LocationID,
	})
	if err != nil {
		return nil, false, err
	}
	response, err := json.Marshal(publicAppointmentRequestResponse{ID: appointment.ID, Status: "requested"})
	if err != nil {
		return nil, false, err
	}
	if err := queries.CompletePublicAppointmentIdempotency(ctx, db.CompletePublicAppointmentIdempotencyParams{
		ResponseBody: response, PracticeID: practice.ID, IdempotencyKey: key,
	}); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return response, false, nil
}

func validatePublicAppointmentResources(ctx context.Context, queries *db.Queries, practice db.GetPublicBookingPracticeRow, input publicAppointmentRequestInput, date time.Time) error {
	if _, err := queries.GetPublicProcedureType(ctx, db.GetPublicProcedureTypeParams{PracticeID: practice.ID, Value: input.AppointmentType}); errors.Is(err, pgx.ErrNoRows) {
		return &schedulingError{Status: http.StatusBadRequest, Message: "Appointment type is unavailable"}
	} else if err != nil {
		return err
	}
	if input.PreferredProviderID != nil {
		if _, err := queries.GetPublicProvider(ctx, db.GetPublicProviderParams{PracticeID: practice.ID, ProviderID: *input.PreferredProviderID}); errors.Is(err, pgx.ErrNoRows) {
			return &schedulingError{Status: http.StatusBadRequest, Message: "Preferred provider is unavailable"}
		} else if err != nil {
			return err
		}
	}
	weekdays := practice.AvailableWeekdays
	if practice.MultipleLocationsEnabled {
		if input.LocationID == nil {
			return &schedulingError{Status: http.StatusBadRequest, Message: "A location is required"}
		}
		locationWeekdays, err := queries.GetPublicBookingLocation(ctx, db.GetPublicBookingLocationParams{ID: *input.LocationID, PracticeID: practice.ID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && len(locationWeekdays) == 0) {
			return &schedulingError{Status: http.StatusBadRequest, Message: "Location is unavailable"}
		}
		if err != nil {
			return err
		}
		weekdays = locationWeekdays
	} else if input.LocationID != nil {
		return &schedulingError{Status: http.StatusBadRequest, Message: "A location cannot be selected for this practice"}
	}
	if !slices.Contains(weekdays, int16(date.Weekday())) {
		return &schedulingError{Status: http.StatusBadRequest, Message: "Requested date is unavailable"}
	}
	return nil
}

func findOrCreatePublicBookingPatient(ctx context.Context, tx pgx.Tx, queries *db.Queries, practiceID uuid.UUID, input publicAppointmentRequestInput) (*uuid.UUID, error) {
	phone := normalizePatientPhone(input.MobilePhone)
	if err := lockPatientContacts(ctx, tx, practiceID, input.Email, phone); err != nil {
		return nil, err
	}
	email := input.Email
	patientID, err := queries.FindPublicBookingPatient(ctx, db.FindPublicBookingPatientParams{
		PracticeID: practiceID, FirstName: input.FirstName, LastName: input.LastName,
		Email: &email, MobilePhone: &phone,
	})
	if err == nil {
		return &patientID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if _, err := queries.PublicBookingContactExists(ctx, db.PublicBookingContactExistsParams{
		PracticeID: practiceID, Email: &email, MobilePhone: &phone,
	}); err == nil {
		return nil, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	patientID, err = queries.CreatePublicBookingPatient(ctx, db.CreatePublicBookingPatientParams{
		PracticeID: practiceID, FirstName: input.FirstName, LastName: input.LastName,
		Email: &email, MobilePhone: &phone,
	})
	if err != nil {
		return nil, err
	}
	return &patientID, nil
}

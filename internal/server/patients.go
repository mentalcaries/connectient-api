package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type PatientSummaryResponse struct {
	ID          uuid.UUID `json:"id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Email       *string   `json:"email"`
	MobilePhone *string   `json:"mobile_phone"`
	DateOfBirth *string   `json:"date_of_birth"`
	CreatedAt   time.Time `json:"created_at"`
}

type PatientResponse struct {
	ID                    uuid.UUID `json:"id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	PracticeID            uuid.UUID `json:"practice_id"`
	FirstName             string    `json:"first_name"`
	LastName              string    `json:"last_name"`
	Email                 *string   `json:"email"`
	MobilePhone           *string   `json:"mobile_phone"`
	HomePhone             *string   `json:"home_phone"`
	DateOfBirth           *string   `json:"date_of_birth"`
	AddressLine1          *string   `json:"address_line_1"`
	AddressLine2          *string   `json:"address_line_2"`
	City                  *string   `json:"city"`
	EmailConsent          bool      `json:"email_consent"`
	WhatsappConsent       bool      `json:"whatsapp_consent"`
	EmergencyContactName  *string   `json:"emergency_contact_name"`
	EmergencyContactPhone *string   `json:"emergency_contact_phone"`
	Notes                 *string   `json:"notes"`
}

type PatientAppointmentResponse struct {
	ID              uuid.UUID  `json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	ModifiedAt      time.Time  `json:"modified_at"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	Email           string     `json:"email"`
	MobilePhone     string     `json:"mobile_phone"`
	RequestedDate   string     `json:"requested_date"`
	RequestedTime   string     `json:"requested_time"`
	IsEmergency     bool       `json:"is_emergency"`
	Description     *string    `json:"description"`
	AppointmentType string     `json:"appointment_type"`
	IsScheduled     bool       `json:"is_scheduled"`
	ScheduledDate   *string    `json:"scheduled_date"`
	ScheduledTime   *string    `json:"scheduled_time"`
	IsConfirmed     bool       `json:"is_confirmed"`
	IsCancelled     bool       `json:"is_cancelled"`
	DurationMinutes *int32     `json:"duration_minutes"`
	ProviderID      *uuid.UUID `json:"provider_id"`
	LocationID      *uuid.UUID `json:"location_id"`
	PatientID       *uuid.UUID `json:"patient_id"`
}

type PatientRegistrationSummary struct {
	ID          uuid.UUID  `json:"id"`
	Status      string     `json:"status"`
	SentAt      *time.Time `json:"sent_at"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type patchPatientInput struct {
	params db.UpdatePatientParams
}

func (input patchPatientInput) hasUpdates() bool {
	p := input.params
	return p.SetFirstName || p.SetLastName || p.SetEmail || p.SetMobilePhone || p.SetHomePhone ||
		p.SetDateOfBirth || p.SetAddressLine1 || p.SetAddressLine2 || p.SetCity || p.SetEmailConsent ||
		p.SetWhatsappConsent || p.SetEmergencyContactName || p.SetEmergencyContactPhone || p.SetNotes
}

func (s *Server) handlerListPatients(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	search := sanitizePatientSearch(c.Query("q"))
	limit := int32(500)
	if search != "" {
		limit = 20
	}
	rows, err := s.DBQuery.ListPatients(c, db.ListPatientsParams{
		PracticeID: *user.PracticeId, Search: search, ResultLimit: limit,
	})
	if err != nil {
		respondPatientError(c, http.StatusInternalServerError, "Failed to fetch patients", err)
		return
	}
	patients := make([]PatientSummaryResponse, 0, len(rows))
	for _, row := range rows {
		patients = append(patients, PatientSummaryResponse{
			ID: row.ID, FirstName: row.FirstName, LastName: row.LastName,
			Email: row.Email, MobilePhone: row.MobilePhone,
			DateOfBirth: formatPatientDate(row.DateOfBirth), CreatedAt: row.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"patients": patients})
}

func (s *Server) handlerGetPatient(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	patientID, ok := parsePatientID(c)
	if !ok {
		return
	}
	patient, err := s.DBQuery.GetPatient(c, db.GetPatientParams{ID: patientID, PracticeID: *user.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondPatientError(c, http.StatusNotFound, "Patient not found", nil)
		return
	}
	if err != nil {
		respondPatientError(c, http.StatusInternalServerError, "Failed to fetch patient", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"patient": patientResponse(patient)})
}

func (s *Server) handlerPatchPatient(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	patientID, ok := parsePatientID(c)
	if !ok {
		return
	}
	input, err := decodePatchPatient(c.Request.Body)
	if err != nil {
		respondPatientError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if !input.hasUpdates() {
		respondPatientError(c, http.StatusBadRequest, "No valid fields to update", nil)
		return
	}
	input.params.ID = patientID
	input.params.PracticeID = *user.PracticeId
	if input.params.SetEmail && input.params.Email != nil {
		_, err = s.DBQuery.FindDuplicatePatientEmail(c, db.FindDuplicatePatientEmailParams{
			PracticeID: *user.PracticeId, ID: patientID, Email: input.params.Email,
		})
		if err == nil {
			respondPatientError(c, http.StatusConflict, "duplicate_email", nil)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			respondPatientError(c, http.StatusInternalServerError, "Failed to update patient", err)
			return
		}
	}
	if input.params.SetMobilePhone && input.params.MobilePhone != nil {
		_, err = s.DBQuery.FindDuplicatePatientMobilePhone(c, db.FindDuplicatePatientMobilePhoneParams{
			PracticeID: *user.PracticeId, ID: patientID, MobilePhone: input.params.MobilePhone,
		})
		if err == nil {
			respondPatientError(c, http.StatusConflict, "duplicate_phone", nil)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			respondPatientError(c, http.StatusInternalServerError, "Failed to update patient", err)
			return
		}
	}
	patient, err := s.DBQuery.UpdatePatient(c, input.params)
	if errors.Is(err, pgx.ErrNoRows) {
		respondPatientError(c, http.StatusNotFound, "Patient not found", nil)
		return
	}
	if err != nil {
		if conflict := patientUniqueConflict(err); conflict != "" {
			respondPatientError(c, http.StatusConflict, conflict, nil)
			return
		}
		respondPatientError(c, http.StatusInternalServerError, "Failed to update patient", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"patient": patientResponse(patient)})
}

func (s *Server) handlerGetPatientAppointments(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	patientID, ok := parsePatientID(c)
	if !ok || !s.ensurePatientOwned(c, patientID, *user.PracticeId) {
		return
	}
	rows, err := s.DBQuery.GetPatientAppointments(c, db.GetPatientAppointmentsParams{
		PatientID: &patientID, PracticeID: *user.PracticeId,
	})
	if err != nil {
		respondPatientError(c, http.StatusInternalServerError, "Failed to fetch patient appointments", err)
		return
	}
	appointments := make([]PatientAppointmentResponse, 0, len(rows))
	for _, row := range rows {
		appointments = append(appointments, PatientAppointmentResponse{
			ID: row.ID, CreatedAt: row.CreatedAt, ModifiedAt: row.ModifiedAt,
			FirstName: row.FirstName, LastName: row.LastName, Email: row.Email,
			MobilePhone: row.MobilePhone, RequestedDate: row.RequestedDate.Format("2006-01-02"),
			RequestedTime: row.RequestedTime, IsEmergency: row.IsEmergency,
			Description: row.Description, AppointmentType: row.AppointmentType,
			IsScheduled: row.IsScheduled, ScheduledDate: formatPatientDate(row.ScheduledDate),
			ScheduledTime: row.ScheduledTime, IsConfirmed: row.IsConfirmed,
			IsCancelled: row.IsCancelled, DurationMinutes: row.DurationMinutes,
			ProviderID: row.ProviderID, LocationID: row.LocationID, PatientID: row.PatientID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": appointments})
}

func (s *Server) handlerGetLatestPatientRegistration(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	patientID, ok := parsePatientID(c)
	if !ok || !s.ensurePatientOwned(c, patientID, *user.PracticeId) {
		return
	}
	row, err := s.DBQuery.GetLatestPatientRegistration(c, db.GetLatestPatientRegistrationParams{
		PatientID: &patientID, PracticeID: *user.PracticeId,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	if err != nil {
		respondPatientError(c, http.StatusInternalServerError, "Failed to fetch patient registration", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": PatientRegistrationSummary{
		ID: row.ID, Status: row.Status, SentAt: row.SentAt,
		CompletedAt: row.CompletedAt, CreatedAt: row.CreatedAt,
	}})
}

func (s *Server) ensurePatientOwned(c *gin.Context, patientID, practiceID uuid.UUID) bool {
	_, err := s.DBQuery.GetPatient(c, db.GetPatientParams{ID: patientID, PracticeID: practiceID})
	if errors.Is(err, pgx.ErrNoRows) {
		respondPatientError(c, http.StatusNotFound, "Patient not found", nil)
		return false
	}
	if err != nil {
		respondPatientError(c, http.StatusInternalServerError, "Failed to fetch patient", err)
		return false
	}
	return true
}

func decodePatchPatient(body io.Reader) (patchPatientInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return patchPatientInput{}, errors.New("Invalid request")
	}
	var input patchPatientInput
	p := &input.params
	if raw, ok := fields["first_name"]; ok {
		value, err := requiredPatientText(raw, "first_name", 100)
		if err != nil {
			return patchPatientInput{}, err
		}
		p.SetFirstName, p.FirstName = true, value
	}
	if raw, ok := fields["last_name"]; ok {
		value, err := requiredPatientText(raw, "last_name", 100)
		if err != nil {
			return patchPatientInput{}, err
		}
		p.SetLastName, p.LastName = true, value
	}
	if raw, ok := fields["email"]; ok {
		value, err := nullablePatientText(raw, "email", 320)
		if err != nil {
			return patchPatientInput{}, err
		}
		if value != nil {
			address, parseErr := mail.ParseAddress(*value)
			if parseErr != nil || address.Address != *value {
				return patchPatientInput{}, errors.New("Invalid email")
			}
		}
		p.SetEmail, p.Email = true, value
	}
	for _, field := range []struct {
		name string
		set  *bool
		to   **string
		max  int
	}{
		{"mobile_phone", &p.SetMobilePhone, &p.MobilePhone, 30},
		{"home_phone", &p.SetHomePhone, &p.HomePhone, 30},
		{"address_line_1", &p.SetAddressLine1, &p.AddressLine1, 200},
		{"address_line_2", &p.SetAddressLine2, &p.AddressLine2, 200},
		{"city", &p.SetCity, &p.City, 100},
		{"emergency_contact_name", &p.SetEmergencyContactName, &p.EmergencyContactName, 100},
		{"emergency_contact_phone", &p.SetEmergencyContactPhone, &p.EmergencyContactPhone, 30},
		{"notes", &p.SetNotes, &p.Notes, 5000},
	} {
		raw, ok := fields[field.name]
		if !ok {
			continue
		}
		value, err := nullablePatientText(raw, field.name, field.max)
		if err != nil {
			return patchPatientInput{}, err
		}
		if strings.Contains(field.name, "phone") && value != nil && utf8.RuneCountInString(*value) < 7 {
			return patchPatientInput{}, errors.New("Invalid " + field.name)
		}
		*field.set, *field.to = true, value
	}
	if raw, ok := fields["date_of_birth"]; ok {
		p.SetDateOfBirth = true
		if !isJSONNull(raw) {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return patchPatientInput{}, errors.New("Invalid date_of_birth")
			}
			value = strings.TrimSpace(value)
			if value != "" {
				date, err := time.Parse("2006-01-02", value)
				if err != nil {
					return patchPatientInput{}, errors.New("Invalid date_of_birth")
				}
				p.DateOfBirth = &date
			}
		}
	}
	if raw, ok := fields["email_consent"]; ok {
		value, err := decodeBoolean(raw, "email_consent")
		if err != nil {
			return patchPatientInput{}, err
		}
		p.SetEmailConsent, p.EmailConsent = true, value
	}
	if raw, ok := fields["whatsapp_consent"]; ok {
		value, err := decodeBoolean(raw, "whatsapp_consent")
		if err != nil {
			return patchPatientInput{}, err
		}
		p.SetWhatsappConsent, p.WhatsappConsent = true, value
	}
	return input, nil
}

func requiredPatientText(raw json.RawMessage, name string, max int) (string, error) {
	if isJSONNull(raw) {
		return "", errors.New("Invalid " + name)
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", errors.New("Invalid " + name)
	}
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > max {
		return "", errors.New("Invalid " + name)
	}
	return value, nil
}

func nullablePatientText(raw json.RawMessage, name string, max int) (*string, error) {
	if isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return nil, errors.New("Invalid " + name)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(value) > max {
		return nil, errors.New("Invalid " + name)
	}
	return &value, nil
}

func sanitizePatientSearch(value string) string {
	replacer := strings.NewReplacer("%", " ", "_", " ", ",", " ", "(", " ", ")", " ")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func parsePatientID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondPatientError(c, http.StatusBadRequest, "Invalid patient ID", nil)
		return uuid.Nil, false
	}
	return id, true
}

func patientResponse(patient db.Patient) PatientResponse {
	return PatientResponse{
		ID: patient.ID, CreatedAt: patient.CreatedAt, UpdatedAt: patient.UpdatedAt,
		PracticeID: patient.PracticeID, FirstName: patient.FirstName, LastName: patient.LastName,
		Email: patient.Email, MobilePhone: patient.MobilePhone, HomePhone: patient.HomePhone,
		DateOfBirth: formatPatientDate(patient.DateOfBirth), AddressLine1: patient.AddressLine1,
		AddressLine2: patient.AddressLine2, City: patient.City, EmailConsent: patient.EmailConsent,
		WhatsappConsent: patient.WhatsappConsent, EmergencyContactName: patient.EmergencyContactName,
		EmergencyContactPhone: patient.EmergencyContactPhone, Notes: patient.Notes,
	}
}

func formatPatientDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02")
	return &formatted
}

func patientUniqueConflict(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return ""
	}
	switch pgErr.ConstraintName {
	case "patients_practice_email_idx":
		return "duplicate_email"
	case "patients_practice_mobile_idx":
		return "duplicate_phone"
	default:
		return ""
	}
}

func respondPatientError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status > 499 {
		log.Printf("Responding with a %v error: %s", status, message)
	}
	c.JSON(status, gin.H{"error": message})
}

func setPrivateNoStore(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
}

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const maxRegistrationSubmissionSize = 1 << 20

type publicRegistrationSubmission struct {
	FormVersion           string
	FormData              json.RawMessage
	FirstName             string
	LastName              string
	Email                 string
	MobilePhone           string
	HomePhone             *string
	DateOfBirth           time.Time
	AddressLine1          *string
	AddressLine2          *string
	City                  *string
	EmailConsent          bool
	EmergencyContactName  *string
	EmergencyContactPhone *string
}

func (s *Server) handlerGetPublicRegistrationForm(c *gin.Context) {
	setPrivateNoStore(c)
	token := c.Param("token")
	if !validRegistrationToken(token) {
		respondRegistrationError(c, http.StatusNotFound, "Token not found", nil)
		return
	}
	row, err := s.DBQuery.GetPublicRegistrationBootstrap(c, token)
	if errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusNotFound, "Token not found", nil)
		return
	}
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to load registration form", err)
		return
	}
	if !publicRegistrationPracticeAllowed(row.IsActive, row.IsSuspended, row.SubscriptionStatus, row.SubscriptionPlan,
		nullableTimestamp(row.SubscriptionTrialEnd), nullableTimestamp(row.SubscriptionPeriodEnd), nullableTimestamp(row.SubscriptionCancelAt)) {
		respondRegistrationError(c, http.StatusNotFound, "Token not found", nil)
		return
	}
	if row.Status == "completed" {
		respondRegistrationError(c, http.StatusConflict, "This form has already been submitted", nil)
		return
	}
	if row.Status == "expired" || row.TokenExpiresAt.Before(time.Now()) {
		_, _ = s.DBQuery.MarkRegistrationExpired(c, row.ID)
		respondRegistrationError(c, http.StatusGone, "This link has expired", nil)
		return
	}
	theme := "default"
	if row.Theme != nil {
		theme = *row.Theme
	}
	c.JSON(http.StatusOK, gin.H{
		"registration_id": row.ID,
		"practice":        gin.H{"name": row.PracticeName, "logo_url": row.PracticeLogo, "practice_category": row.PracticeCategory},
		"form_config": gin.H{
			"dental_history_enabled":        boolValue(row.DentalHistoryEnabled),
			"tmj_history_enabled":           boolValue(row.TmjHistoryEnabled),
			"physiotherapy_history_enabled": boolValue(row.PhysiotherapyHistoryEnabled),
			"optometry_history_enabled":     boolValue(row.OptometryHistoryEnabled),
			"custom_form_sections":          nullableJSON(row.CustomFormSections),
		},
		"theme_settings": gin.H{"theme": theme, "theme_colors": nullableJSON(row.ThemeColors)},
		"prefill": gin.H{
			"first_name": row.FirstName, "last_name": row.LastName,
			"email": row.Email, "mobile_phone": row.MobilePhone,
		},
	})
}

func (s *Server) handlerSubmitPublicRegistrationForm(c *gin.Context) {
	setPrivateNoStore(c)
	token := c.Param("token")
	if !validRegistrationToken(token) {
		respondRegistrationError(c, http.StatusNotFound, "Token not found", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRegistrationSubmissionSize)
	input, err := decodePublicRegistrationSubmission(c.Request.Body)
	if err != nil {
		respondRegistrationError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	registration, err := queries.LockPublicRegistration(c, token)
	if errors.Is(err, pgx.ErrNoRows) {
		respondRegistrationError(c, http.StatusNotFound, "Token not found", nil)
		return
	}
	if err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
		return
	}
	if !publicRegistrationPracticeAllowed(registration.IsActive, registration.IsSuspended,
		registration.SubscriptionStatus, registration.SubscriptionPlan,
		nullableTimestamp(registration.SubscriptionTrialEnd), nullableTimestamp(registration.SubscriptionPeriodEnd),
		nullableTimestamp(registration.SubscriptionCancelAt)) {
		respondRegistrationError(c, http.StatusNotFound, "Token not found", nil)
		return
	}
	if registration.Status == "completed" {
		respondRegistrationError(c, http.StatusConflict, "This form has already been submitted", nil)
		return
	}
	if registration.Status == "expired" || registration.TokenExpiresAt.Before(time.Now()) {
		if _, err := queries.MarkRegistrationExpired(c, registration.ID); err != nil {
			respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
			return
		}
		if err := tx.Commit(c); err != nil {
			respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
			return
		}
		respondRegistrationError(c, http.StatusGone, "This link has expired", nil)
		return
	}
	if err := lockRegistrationPatientContacts(c, tx, registration.PracticeID, input.Email, input.MobilePhone); err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
		return
	}
	patientID, err := findOrCreateRegistrationPatient(c, queries, registration.PracticeID, input)
	if err != nil {
		if isUniqueViolation(err) {
			respondRegistrationError(c, http.StatusConflict, "Patient contact already exists", nil)
			return
		}
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
		return
	}
	rows, err := queries.EnrichRegistrationPatient(c, db.EnrichRegistrationPatientParams{
		HomePhone: input.HomePhone, DateOfBirth: &input.DateOfBirth,
		AddressLine1: input.AddressLine1, AddressLine2: input.AddressLine2, City: input.City,
		EmailConsent: input.EmailConsent, EmergencyContactName: input.EmergencyContactName,
		EmergencyContactPhone: input.EmergencyContactPhone, ID: patientID, PracticeID: registration.PracticeID,
	})
	if err != nil || rows != 1 {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to submit registration", err)
		return
	}
	if err := queries.InsertPatientRegistrationData(c, db.InsertPatientRegistrationDataParams{
		RegistrationID: registration.ID, FormVersion: input.FormVersion, FormData: input.FormData,
	}); err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to save form data", err)
		return
	}
	rows, err = queries.CompletePatientRegistration(c, db.CompletePatientRegistrationParams{
		PatientID: &patientID, ID: registration.ID,
	})
	if err != nil || rows != 1 {
		respondRegistrationError(c, http.StatusConflict, "This form has already been submitted", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondRegistrationError(c, http.StatusInternalServerError, "Failed to complete registration", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Registration submitted successfully."})
}

func decodePublicRegistrationSubmission(body io.Reader) (publicRegistrationSubmission, error) {
	var envelope struct {
		FormVersion string          `json:"form_version"`
		FormData    json.RawMessage `json:"form_data"`
	}
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&envelope); err != nil || ensureJSONEnd(decoder) != nil {
		return publicRegistrationSubmission{}, errors.New("Invalid request body")
	}
	envelope.FormVersion = strings.TrimSpace(envelope.FormVersion)
	if envelope.FormVersion == "" || utf8.RuneCountInString(envelope.FormVersion) > 100 || len(envelope.FormData) == 0 || isJSONNull(envelope.FormData) {
		return publicRegistrationSubmission{}, errors.New("form_data and form_version are required")
	}
	var sections map[string]json.RawMessage
	if json.Unmarshal(envelope.FormData, &sections) != nil || sections == nil {
		return publicRegistrationSubmission{}, errors.New("Invalid form_data")
	}
	personal, err := decodeRegistrationSection(sections["personal"], "personal")
	if err != nil {
		return publicRegistrationSubmission{}, err
	}
	additional, err := decodeRegistrationSection(sections["additional"], "additional")
	if err != nil {
		return publicRegistrationSubmission{}, err
	}
	if _, err := decodeRegistrationSection(sections["medical_history"], "medical_history"); err != nil {
		return publicRegistrationSubmission{}, err
	}
	signature, err := decodeRegistrationSection(sections["signature"], "signature")
	if err != nil {
		return publicRegistrationSubmission{}, err
	}
	input := publicRegistrationSubmission{FormVersion: envelope.FormVersion, FormData: envelope.FormData}
	if input.LastName, err = requiredRegistrationField(personal, "surname", 100); err != nil {
		return input, err
	}
	if input.FirstName, err = requiredRegistrationField(personal, "first_name", 100); err != nil {
		return input, err
	}
	date, err := requiredRegistrationField(personal, "date_of_birth", 10)
	if err != nil {
		return input, err
	}
	input.DateOfBirth, err = time.Parse("2006-01-02", date)
	if err != nil {
		return input, errors.New("Invalid personal.date_of_birth")
	}
	for _, field := range []string{"sex"} {
		if _, err := requiredRegistrationField(personal, field, 100); err != nil {
			return input, err
		}
	}
	address, err := requiredRegistrationField(personal, "address_line_1", 200)
	if err != nil {
		return input, err
	}
	input.AddressLine1 = &address
	input.AddressLine2 = optionalRegistrationField(personal, "address_line_2", 200)
	input.City = optionalRegistrationField(personal, "city", 100)
	email, err := requiredRegistrationField(personal, "email", 320)
	if err != nil {
		return input, err
	}
	email = strings.ToLower(email)
	addressValue, err := mail.ParseAddress(email)
	if err != nil || addressValue.Address != email {
		return input, errors.New("Invalid personal.email")
	}
	input.Email = email
	phone, err := requiredRegistrationField(personal, "mobile_phone", 30)
	if err != nil {
		return input, err
	}
	input.MobilePhone = digitsOnly(phone)
	if len(input.MobilePhone) < 8 || len(input.MobilePhone) > 15 {
		return input, errors.New("Invalid personal.mobile_phone")
	}
	input.HomePhone = optionalRegistrationField(personal, "home_phone", 30)
	if raw, ok := personal["email_contact_ok"]; ok {
		if json.Unmarshal(raw, &input.EmailConsent) != nil {
			return input, errors.New("Invalid personal.email_contact_ok")
		}
	} else {
		return input, errors.New("Invalid personal.email_contact_ok")
	}
	if _, err := requiredRegistrationField(additional, "marital_status", 100); err != nil {
		return input, err
	}
	emergencyName, err := requiredRegistrationField(additional, "emergency_contact_name", 100)
	if err != nil {
		return input, err
	}
	input.EmergencyContactName = &emergencyName
	emergencyPhone, err := requiredRegistrationField(additional, "emergency_contact_phone", 30)
	if err != nil {
		return input, err
	}
	input.EmergencyContactPhone = &emergencyPhone
	if _, err := requiredRegistrationField(signature, "signed_by", 200); err != nil {
		return input, err
	}
	return input, nil
}

func decodeRegistrationSection(raw json.RawMessage, name string) (map[string]json.RawMessage, error) {
	var section map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &section) != nil || section == nil {
		return nil, errors.New("Invalid " + name)
	}
	return section, nil
}

func requiredRegistrationField(section map[string]json.RawMessage, name string, max int) (string, error) {
	raw, ok := section[name]
	if !ok {
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

func optionalRegistrationField(section map[string]json.RawMessage, name string, max int) *string {
	raw, ok := section[name]
	if !ok {
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

func findOrCreateRegistrationPatient(c *gin.Context, queries *db.Queries, practiceID uuid.UUID, input publicRegistrationSubmission) (uuid.UUID, error) {
	email := input.Email
	patientID, err := queries.FindRegistrationPatientByEmail(c, db.FindRegistrationPatientByEmailParams{
		PracticeID: practiceID, Email: &email, FirstName: input.FirstName, LastName: input.LastName,
	})
	if err == nil {
		return patientID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	phone := input.MobilePhone
	patientID, err = queries.FindRegistrationPatientByPhone(c, db.FindRegistrationPatientByPhoneParams{
		PracticeID: practiceID, MobilePhone: &phone, FirstName: input.FirstName, LastName: input.LastName,
	})
	if err == nil {
		return patientID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	return queries.CreateRegistrationPatient(c, db.CreateRegistrationPatientParams{
		PracticeID: practiceID, FirstName: input.FirstName, LastName: input.LastName,
		Email: &email, MobilePhone: &phone,
	})
}

func lockRegistrationPatientContacts(c *gin.Context, tx pgx.Tx, practiceID uuid.UUID, email, phone string) error {
	keys := []string{
		practiceID.String() + ":patient:" + email,
		practiceID.String() + ":patient:" + phone,
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := tx.Exec(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
			return err
		}
	}
	return nil
}

func publicRegistrationPracticeAllowed(active, suspended bool, status, plan *string, trialEnd, periodEnd, cancelAt *time.Time) bool {
	if !active || suspended {
		return false
	}
	return computeSubscriptionContext(status, plan, trialEnd, periodEnd, cancelAt, time.Now()).CanAccessRegistrations
}

func validRegistrationToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	for _, char := range token {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return json.RawMessage(value)
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

var nonPhoneDigits = regexp.MustCompile(`\D`)

type schedulingError struct {
	Status  int
	Message string
}

func (e *schedulingError) Error() string { return e.Message }

type existingPatientError struct {
	Patient patientSummary
}

func (e *existingPatientError) Error() string {
	return "A patient with this contact information already exists"
}

type patientSummary struct {
	ID          uuid.UUID `json:"id"`
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name"`
	Email       *string   `json:"email"`
	MobilePhone *string   `json:"mobile_phone"`
}

type staffAppointmentPatientInput struct {
	Kind        string
	ID          uuid.UUID
	FirstName   string
	LastName    string
	Email       *string
	MobilePhone string
}

type createStaffAppointmentInput struct {
	Patient                 staffAppointmentPatientInput
	AppointmentType         string
	ScheduledDate           string
	ScheduledTime           string
	LocationID              *uuid.UUID
	DurationMinutes         int
	ProviderID              uuid.UUID
	AcknowledgedConflictIDs []uuid.UUID
	IsConfirmed             bool
	SendEmail               bool
	SendWhatsApp            bool
	Timezone                string
}

type optionalNullableUUID struct {
	Set   bool
	Value *uuid.UUID
}

func (value *optionalNullableUUID) UnmarshalJSON(data []byte) error {
	value.Set = true
	if isJSONNull(data) {
		value.Value = nil
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	id, err := uuid.Parse(text)
	if err != nil {
		return err
	}
	value.Value = &id
	return nil
}

type optionalNullableString struct {
	Set   bool
	Value *string
}

type optionalStringInput struct {
	Set   bool
	Value string
}

func (value *optionalStringInput) UnmarshalJSON(data []byte) error {
	value.Set = true
	if isJSONNull(data) {
		return errors.New("null is not allowed")
	}
	return json.Unmarshal(data, &value.Value)
}

type optionalBoolInput struct {
	Set   bool
	Value bool
}

func (value *optionalBoolInput) UnmarshalJSON(data []byte) error {
	value.Set = true
	if isJSONNull(data) {
		return errors.New("null is not allowed")
	}
	return json.Unmarshal(data, &value.Value)
}

type optionalStringListInput struct {
	Set    bool
	Values []string
}

func (value *optionalStringListInput) UnmarshalJSON(data []byte) error {
	value.Set = true
	if isJSONNull(data) {
		return errors.New("null is not allowed")
	}
	return json.Unmarshal(data, &value.Values)
}

func (value *optionalNullableString) UnmarshalJSON(data []byte) error {
	value.Set = true
	if isJSONNull(data) {
		value.Value = nil
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	value.Value = &text
	return nil
}

type scheduleAppointmentInput struct {
	ScheduledDate           string
	ScheduledTime           string
	Location                optionalNullableUUID
	AppointmentType         optionalNullableString
	DurationMinutes         int
	ProviderID              uuid.UUID
	AcknowledgedConflictIDs []uuid.UUID
	IsConfirmed             bool
	SendWhatsApp            *bool
	NotifyPhone             *string
	Timezone                string
}

type scheduleTransactionResult struct {
	Previous    db.LockAppointmentForSchedulingRow
	Scheduled   bool
	Appointment db.Appointment
	Conflicts   []availabilityConflict
}

func decodeCreateStaffAppointment(body io.Reader) (createStaffAppointmentInput, error) {
	var wire struct {
		Patient                 json.RawMessage         `json:"patient"`
		AppointmentType         string                  `json:"appointmentType"`
		ScheduledDate           string                  `json:"scheduledDate"`
		ScheduledTime           string                  `json:"scheduledTime"`
		Location                optionalNullableUUID    `json:"locationId"`
		DurationMinutes         int                     `json:"durationMinutes"`
		ProviderID              string                  `json:"providerId"`
		AcknowledgedConflictIDs optionalStringListInput `json:"acknowledgedConflictIds"`
		IsConfirmed             optionalBoolInput       `json:"isConfirmed"`
		SendEmail               optionalBoolInput       `json:"sendEmail"`
		SendWhatsApp            optionalBoolInput       `json:"sendWhatsApp"`
		Timezone                optionalStringInput     `json:"timezone"`
	}
	if err := decodeStrictJSON(body, &wire); err != nil {
		return createStaffAppointmentInput{}, err
	}
	patient, err := decodeStaffAppointmentPatient(wire.Patient)
	if err != nil {
		return createStaffAppointmentInput{}, err
	}
	appointmentType := strings.TrimSpace(wire.AppointmentType)
	if appointmentType == "" || utf8.RuneCountInString(appointmentType) > 150 {
		return createStaffAppointmentInput{}, errors.New("invalid appointment type")
	}
	providerID, err := uuid.Parse(wire.ProviderID)
	if err != nil {
		return createStaffAppointmentInput{}, err
	}
	acknowledged, err := parseUUIDList(wire.AcknowledgedConflictIDs.Values)
	if err != nil {
		return createStaffAppointmentInput{}, err
	}
	timezone := "UTC"
	if wire.Timezone.Set {
		timezone = wire.Timezone.Value
	}
	if timezone == "" || len(timezone) > 100 {
		return createStaffAppointmentInput{}, errors.New("invalid timezone")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return createStaffAppointmentInput{}, errors.New("invalid timezone")
	}
	input := createStaffAppointmentInput{
		Patient: patient, AppointmentType: appointmentType, ScheduledDate: wire.ScheduledDate,
		ScheduledTime: wire.ScheduledTime, LocationID: wire.Location.Value,
		DurationMinutes: wire.DurationMinutes, ProviderID: providerID,
		AcknowledgedConflictIDs: acknowledged, IsConfirmed: wire.IsConfirmed.Value,
		SendEmail: true, SendWhatsApp: true, Timezone: timezone,
	}
	if wire.SendEmail.Set {
		input.SendEmail = wire.SendEmail.Value
	}
	if wire.SendWhatsApp.Set {
		input.SendWhatsApp = wire.SendWhatsApp.Value
	}
	if _, err := validateScheduleValues(input.ScheduledDate, input.ScheduledTime, input.DurationMinutes); err != nil {
		return createStaffAppointmentInput{}, err
	}
	return input, nil
}

func decodeScheduleAppointment(body io.Reader) (scheduleAppointmentInput, error) {
	var wire struct {
		ScheduledDate           string                  `json:"scheduledDate"`
		ScheduledTime           string                  `json:"scheduledTime"`
		Location                optionalNullableUUID    `json:"locationId"`
		AppointmentType         optionalNullableString  `json:"appointmentType"`
		DurationMinutes         int                     `json:"durationMinutes"`
		ProviderID              string                  `json:"providerId"`
		AcknowledgedConflictIDs optionalStringListInput `json:"acknowledgedConflictIds"`
		IsConfirmed             optionalBoolInput       `json:"isConfirmed"`
		SendWhatsApp            optionalBoolInput       `json:"sendWhatsApp"`
		NotifyPhone             optionalStringInput     `json:"notifyPhone"`
		Timezone                optionalStringInput     `json:"timezone"`
	}
	if err := decodeStrictJSON(body, &wire); err != nil {
		return scheduleAppointmentInput{}, err
	}
	providerID, err := uuid.Parse(wire.ProviderID)
	if err != nil {
		return scheduleAppointmentInput{}, err
	}
	acknowledged, err := parseUUIDList(wire.AcknowledgedConflictIDs.Values)
	if err != nil {
		return scheduleAppointmentInput{}, err
	}
	timezone := "UTC"
	if wire.Timezone.Set {
		timezone = wire.Timezone.Value
	}
	if timezone == "" || len(timezone) > 100 {
		return scheduleAppointmentInput{}, errors.New("invalid timezone")
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return scheduleAppointmentInput{}, errors.New("invalid timezone")
	}
	input := scheduleAppointmentInput{
		ScheduledDate: wire.ScheduledDate, ScheduledTime: wire.ScheduledTime,
		Location: wire.Location, AppointmentType: wire.AppointmentType,
		DurationMinutes: wire.DurationMinutes, ProviderID: providerID,
		AcknowledgedConflictIDs: acknowledged, IsConfirmed: wire.IsConfirmed.Value,
		Timezone: timezone,
	}
	if wire.SendWhatsApp.Set {
		value := wire.SendWhatsApp.Value
		input.SendWhatsApp = &value
	}
	if wire.NotifyPhone.Set {
		value := wire.NotifyPhone.Value
		input.NotifyPhone = &value
	}
	if _, err := validateScheduleValues(input.ScheduledDate, input.ScheduledTime, input.DurationMinutes); err != nil {
		return scheduleAppointmentInput{}, err
	}
	return input, nil
}

func decodeStaffAppointmentPatient(raw json.RawMessage) (staffAppointmentPatientInput, error) {
	if len(raw) == 0 || isJSONNull(raw) {
		return staffAppointmentPatientInput{}, errors.New("patient is required")
	}
	var discriminator struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(raw, &discriminator) != nil {
		return staffAppointmentPatientInput{}, errors.New("invalid patient")
	}
	switch discriminator.Kind {
	case "existing":
		var wire struct {
			Kind        string              `json:"kind"`
			ID          string              `json:"id"`
			Email       optionalStringInput `json:"email"`
			MobilePhone string              `json:"mobilePhone"`
		}
		if err := decodeStrictJSON(strings.NewReader(string(raw)), &wire); err != nil {
			return staffAppointmentPatientInput{}, err
		}
		id, err := uuid.Parse(wire.ID)
		if err != nil {
			return staffAppointmentPatientInput{}, err
		}
		email, err := validateOptionalEmail(optionalStringPointer(wire.Email))
		if err != nil {
			return staffAppointmentPatientInput{}, err
		}
		phone, err := validatePhone(wire.MobilePhone)
		if err != nil {
			return staffAppointmentPatientInput{}, err
		}
		return staffAppointmentPatientInput{Kind: wire.Kind, ID: id, Email: email, MobilePhone: phone}, nil
	case "new":
		var wire struct {
			Kind        string              `json:"kind"`
			FirstName   string              `json:"firstName"`
			LastName    string              `json:"lastName"`
			Email       optionalStringInput `json:"email"`
			MobilePhone string              `json:"mobilePhone"`
		}
		if err := decodeStrictJSON(strings.NewReader(string(raw)), &wire); err != nil {
			return staffAppointmentPatientInput{}, err
		}
		wire.FirstName, wire.LastName = strings.TrimSpace(wire.FirstName), strings.TrimSpace(wire.LastName)
		if wire.FirstName == "" || wire.LastName == "" || utf8.RuneCountInString(wire.FirstName) > 100 || utf8.RuneCountInString(wire.LastName) > 100 {
			return staffAppointmentPatientInput{}, errors.New("invalid patient name")
		}
		email, err := validateOptionalEmail(optionalStringPointer(wire.Email))
		if err != nil {
			return staffAppointmentPatientInput{}, err
		}
		phone, err := validatePhone(wire.MobilePhone)
		if err != nil {
			return staffAppointmentPatientInput{}, err
		}
		return staffAppointmentPatientInput{
			Kind: wire.Kind, FirstName: wire.FirstName, LastName: wire.LastName,
			Email: email, MobilePhone: phone,
		}, nil
	default:
		return staffAppointmentPatientInput{}, errors.New("invalid patient kind")
	}
}

func decodeStrictJSON(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return ensureJSONEnd(decoder)
}

func validateScheduleValues(dateValue, timeValue string, duration int) (time.Time, error) {
	date, err := time.Parse("2006-01-02", dateValue)
	if err != nil {
		return time.Time{}, &schedulingError{Status: 400, Message: "Invalid appointment date"}
	}
	if duration < 15 || duration > 120 || duration%15 != 0 {
		return time.Time{}, &schedulingError{Status: 400, Message: "Invalid appointment duration"}
	}
	start, ok := parseExactScheduleTime(timeValue)
	if !ok || start < availabilityDayStartMinutes || start%15 != 0 || start+duration > availabilityDayEndMinutes {
		return time.Time{}, &schedulingError{Status: 400, Message: "The selected time is outside scheduling hours"}
	}
	return date, nil
}

func parseExactScheduleTime(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

func validateOptionalEmail(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	address, err := mail.ParseAddress(normalized)
	if err != nil || address.Address != normalized {
		return nil, errors.New("invalid email")
	}
	return &normalized, nil
}

func validatePhone(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 7 || len(trimmed) > 30 {
		return "", errors.New("A complete mobile number is required")
	}
	return trimmed, nil
}

func optionalStringPointer(value optionalStringInput) *string {
	if !value.Set {
		return nil
	}
	result := value.Value
	return &result
}

func parseUUIDList(values []string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}

func normalizePatientPhone(value string) string { return nonPhoneDigits.ReplaceAllString(value, "") }

func scheduleConflicts(rows []db.ListSchedulingConflictsRow) []availabilityConflict {
	result := make([]availabilityConflict, 0, len(rows))
	for _, row := range rows {
		result = append(result, availabilityConflict{
			ID: row.ID, PatientName: strings.TrimSpace(row.FirstName + " " + row.LastName),
			AppointmentType: row.AppointmentType,
			Start:           row.ScheduledDate + "T" + row.StartTime,
			End:             row.ScheduledDate + "T" + row.EndTime,
		})
	}
	return result
}

func unacknowledgedConflicts(conflicts []availabilityConflict, acknowledged []uuid.UUID) bool {
	set := make(map[uuid.UUID]bool, len(acknowledged))
	for _, id := range acknowledged {
		set[id] = true
	}
	for _, conflict := range conflicts {
		if !set[conflict.ID] {
			return true
		}
	}
	return false
}

func lockAppointmentScheduleNamespace(ctx context.Context, tx pgx.Tx, providerID uuid.UUID, date string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, providerID.String()+":"+date)
	return err
}

func lockPatientContacts(ctx context.Context, tx pgx.Tx, practiceID uuid.UUID, contacts ...string) error {
	filtered := make([]string, 0, len(contacts))
	for _, contact := range contacts {
		if contact != "" {
			filtered = append(filtered, contact)
		}
	}
	sort.Strings(filtered)
	for _, contact := range filtered {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, practiceID.String()+":patient:"+contact); err != nil {
			return err
		}
	}
	return nil
}

func validateSchedulingResources(ctx context.Context, queries *db.Queries, practiceID, providerID uuid.UUID, locationID *uuid.UUID, scheduledDate time.Time) error {
	if _, err := queries.GetPracticeProviderLink(ctx, db.GetPracticeProviderLinkParams{PracticeID: practiceID, ProviderID: providerID}); errors.Is(err, pgx.ErrNoRows) {
		return &schedulingError{Status: 400, Message: "Provider is not linked to practice"}
	} else if err != nil {
		return err
	}
	settings, err := queries.GetSchedulingSettings(ctx, practiceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &schedulingError{Status: 400, Message: "Practice availability is unavailable"}
	}
	if err != nil {
		return err
	}
	weekdays := settings.AvailableWeekdays
	if settings.MultipleLocationsEnabled {
		if locationID == nil {
			return &schedulingError{Status: 400, Message: "A location is required"}
		}
		weekdays, err = queries.GetSchedulingLocation(ctx, db.GetSchedulingLocationParams{ID: *locationID, PracticeID: practiceID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && len(weekdays) == 0) {
			return &schedulingError{Status: 400, Message: "The selected location is unavailable"}
		}
		if err != nil {
			return err
		}
	} else if locationID != nil {
		return &schedulingError{Status: 400, Message: "A location cannot be selected for this practice"}
	}
	weekday := int16(scheduledDate.Weekday())
	for _, available := range weekdays {
		if available == weekday {
			return nil
		}
	}
	return &schedulingError{Status: 400, Message: "The selected date is unavailable"}
}

func schedulingErrorResponse(err error, fallback string) (int, string) {
	var domainError *schedulingError
	if errors.As(err, &domainError) {
		return domainError.Status, domainError.Message
	}
	return 500, fallback
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

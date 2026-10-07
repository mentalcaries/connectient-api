package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type appointmentDTO struct {
	ID              uuid.UUID  `json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	ModifiedAt      time.Time  `json:"modified_at"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	Email           string     `json:"email"`
	MobilePhone     string     `json:"mobile_phone"`
	RequestedDate   *string    `json:"requested_date"`
	RequestedTime   string     `json:"requested_time"`
	IsEmergency     bool       `json:"is_emergency"`
	Description     *string    `json:"description"`
	AppointmentType *string    `json:"appointment_type"`
	IsScheduled     bool       `json:"is_scheduled"`
	ScheduledDate   *string    `json:"scheduled_date"`
	ScheduledTime   *string    `json:"scheduled_time"`
	IsCancelled     bool       `json:"is_cancelled"`
	IsConfirmed     bool       `json:"is_confirmed"`
	DurationMinutes *int32     `json:"duration_minutes"`
	CreatedBy       *uuid.UUID `json:"created_by"`
	ScheduledBy     *uuid.UUID `json:"scheduled_by"`
	PracticeID      uuid.UUID  `json:"practice_id"`
	ProviderID      *uuid.UUID `json:"provider_id"`
	LocationID      *uuid.UUID `json:"location_id"`
	PatientID       *uuid.UUID `json:"patient_id"`
	DeletedAt       *time.Time `json:"deleted_at"`
}

type patchAppointmentContactsInput struct {
	SetEmail       bool
	Email          string
	SetMobilePhone bool
	MobilePhone    string
}

func (s *Server) handlerGetAllAppointments(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}
	appointments, err := s.DBQuery.GetAppointments(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not get appointments", err)
		return
	}
	data := make([]appointmentDTO, 0, len(appointments))
	for _, appointment := range appointments {
		data = append(data, appointmentResponse(appointment))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (s *Server) handlerGetConfirmedAppointments(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}
	startDate, endDate, ok := parseAppointmentDateRange(c)
	if !ok {
		return
	}
	appointments, err := s.DBQuery.GetConfirmedAppointments(c, db.GetConfirmedAppointmentsParams{
		PracticeID: *user.PracticeId, StartDate: &startDate, EndDate: &endDate,
	})
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch appointments", err)
		return
	}
	data := make([]appointmentDTO, 0, len(appointments))
	for _, appointment := range appointments {
		data = append(data, appointmentResponse(appointment))
	}
	c.JSON(http.StatusOK, data)
}

func (s *Server) handlerGetAppointmentById(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}
	id, err := parseId(c, "id")
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}
	appointment, err := s.DBQuery.GetAppointmentById(c, db.GetAppointmentByIdParams{ID: id, PracticeID: *user.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		respondWithError(c, http.StatusNotFound, "Appointment not found", nil)
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not get appointment", err)
		return
	}
	c.JSON(http.StatusOK, appointmentResponse(appointment))
}

func (s *Server) handlerAppointmentsUpdate(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}
	id, err := parseId(c, "id")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment ID"})
		return
	}
	input, err := decodePatchAppointmentContacts(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	rows, err := s.DBQuery.PatchAppointmentContacts(c, db.PatchAppointmentContactsParams{
		SetEmail: input.SetEmail, Email: input.Email, SetMobilePhone: input.SetMobilePhone,
		MobilePhone: input.MobilePhone, ID: id, PracticeID: *user.PracticeId,
	})
	if err != nil {
		respondTeamError(c, http.StatusInternalServerError, "Failed to update appointment", err)
		return
	}
	if rows != 1 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func decodePatchAppointmentContacts(body io.Reader) (patchAppointmentContactsInput, error) {
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&fields); err != nil || fields == nil || ensureJSONEnd(decoder) != nil {
		return patchAppointmentContactsInput{}, errors.New("Invalid request body")
	}
	var input patchAppointmentContactsInput
	if raw, ok := fields["email"]; ok {
		if json.Unmarshal(raw, &input.Email) == nil {
			input.SetEmail, input.Email = true, strings.TrimSpace(input.Email)
		}
	}
	if raw, ok := fields["mobile_phone"]; ok {
		if json.Unmarshal(raw, &input.MobilePhone) == nil {
			input.SetMobilePhone, input.MobilePhone = true, strings.TrimSpace(input.MobilePhone)
		}
	}
	if !input.SetEmail && !input.SetMobilePhone {
		return input, errors.New("No valid fields to update")
	}
	return input, nil
}

func parseAppointmentDateRange(c *gin.Context) (time.Time, time.Time, bool) {
	startValue, endValue := c.Query("start"), c.Query("end")
	if startValue == "" || endValue == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing start or end date parameter"})
		return time.Time{}, time.Time{}, false
	}
	start, err := time.Parse("2006-01-02", startValue)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start or end date parameter"})
		return time.Time{}, time.Time{}, false
	}
	end, err := time.Parse("2006-01-02", endValue)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start or end date parameter"})
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

func appointmentResponse(appointment db.Appointment) appointmentDTO {
	return appointmentDTO{
		ID: appointment.ID, CreatedAt: appointment.CreatedAt, ModifiedAt: appointment.ModifiedAt,
		FirstName: appointment.FirstName, LastName: appointment.LastName, Email: appointment.Email,
		MobilePhone: appointment.MobilePhone, RequestedDate: dateString(appointment.RequestedDate),
		RequestedTime: appointment.RequestedTime, IsEmergency: appointment.IsEmergency,
		Description: appointment.Description, AppointmentType: appointment.AppointmentType,
		IsScheduled: appointment.IsScheduled, ScheduledDate: dateString(appointment.ScheduledDate),
		ScheduledTime: appointment.ScheduledTime, IsCancelled: appointment.IsCancelled,
		IsConfirmed: appointment.IsConfirmed, DurationMinutes: appointment.DurationMinutes,
		CreatedBy: appointment.CreatedBy, ScheduledBy: appointment.ScheduledBy,
		PracticeID: appointment.PracticeID, ProviderID: appointment.ProviderID,
		LocationID: appointment.LocationID, PatientID: appointment.PatientID, DeletedAt: appointment.DeletedAt,
	}
}

func dateString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02")
	return &formatted
}

package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type Appointment struct {
	ID              uuid.UUID  `json:"id"`
	CreatedAt       time.Time  `json:"created_at"`
	ModifiedAt      time.Time  `json:"modified_at"`
	Email           string     `json:"email"`
	FirstName       string     `json:"first_name"`
	LastName        string     `json:"last_name"`
	MobilePhone     string     `json:"mobile_phone"`
	RequestedDate   time.Time  `json:"requested_date"`
	IsEmergency     bool       `json:"is_emergency"`
	Description     *string    `json:"description,omitempty"`
	AppointmentType *string    `json:"appointment_type"`
	IsScheduled     *bool      `json:"is_scheduled"`
	ScheduledDate   *time.Time `json:"scheduled_date,omitempty"`
	ScheduledTime   *string    `json:"scheduled_time,omitempty"`
	CreatedBy       *uuid.UUID `json:"created_by,omitempty"`
	ScheduledBy     *uuid.UUID `json:"scheduled_by,omitempty"`
	IsCancelled     *bool      `json:"is_cancelled"`
	RequestedTime   string     `json:"requested_time"`
	PracticeID      uuid.UUID  `json:"practice_id"`
	Token           string     `json:"token"`
}

type NewAppointmentRequest struct {
	Email           string    `json:"email"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	MobilePhone     string    `json:"mobile_phone"`
	RequestedDate   string    `json:"requested_date"`
	IsEmergency     bool      `json:"is_emergency"`
	Description     string    `json:"description,omitempty"`
	AppointmentType string    `json:"appointment_type,omitempty"`
	RequestedTime   string    `json:"requested_time"`
	PracticeID      uuid.UUID `json:"practice_id"`
}

type UpdateAppointmentRequest struct {
	ID            uuid.UUID  `json:"id"`
	ScheduledDate *time.Time `json:"scheduled_date,omitempty"`
	ScheduledTime *string    `json:"scheduled_time,omitempty"`
	IsScheduled   bool       `json:"is_scheduled,omitempty"`
	IsCancelled   bool       `json:"is_cancelled,omitempty"`
}

type ConfirmedAppointment struct {
	ID              uuid.UUID `json:"id"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	AppointmentType string    `json:"appointment_type"`
	MobilePhone     string    `json:"mobile_phone"`
	ScheduledDate   time.Time `json:"scheduled_date"`
	ScheduledTime   *string   `json:"scheduled_time"`
	DurationMinutes *int      `json:"duration_minutes"`
}

func (s *Server) handlerGetAllAppointments(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	dbAppts, err := s.DBQuery.GetAppointments(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not get Appointments", err)
		return
	}
	appointments := []Appointment{}

	for _, dbAppt := range dbAppts {
		appointments = append(appointments, Appointment{
			ID:              dbAppt.ID,
			CreatedAt:       dbAppt.CreatedAt,
			ModifiedAt:      dbAppt.ModifiedAt,
			Email:           dbAppt.Email,
			FirstName:       dbAppt.FirstName,
			LastName:        dbAppt.LastName,
			MobilePhone:     dbAppt.MobilePhone,
			RequestedDate:   dbAppt.RequestedDate,
			IsEmergency:     dbAppt.IsEmergency,
			Description:     dbAppt.Description,
			AppointmentType: &dbAppt.AppointmentType,
			IsScheduled:     &dbAppt.IsScheduled,
			ScheduledDate:   dbAppt.ScheduledDate,
			CreatedBy:       dbAppt.CreatedBy,
			ScheduledBy:     dbAppt.ScheduledBy,
			IsCancelled:     &dbAppt.IsCancelled,
			RequestedTime:   dbAppt.RequestedTime,
			ScheduledTime:   dbAppt.ScheduledTime,
			PracticeID:      dbAppt.PracticeID,
			Token:           dbAppt.Token,
		})
	}
	c.JSON(http.StatusOK, appointments)
}

func (s *Server) handlerGetConfirmedAppointments(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	startDate, err := time.Parse("2006-01-02", c.Query("start"))
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "invalid start date", err)
		return
	}
	endDate, err := time.Parse("2006-01-02", c.Query("end"))
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "invalid start date", err)
		return
	}
	dbAppts, err := s.DBQuery.GetConfirmedAppointments(c, db.GetConfirmedAppointmentsParams{
		PracticeID: *user.PracticeId,
		StartDate:  &startDate,
		EndDate:    &endDate,
	})
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not get Appointments", err)
		return
	}
	appointments := []ConfirmedAppointment{}

	for _, dbAppt := range dbAppts {
		appointments = append(appointments, ConfirmedAppointment{
			ID:              dbAppt.ID,
			FirstName:       dbAppt.FirstName,
			LastName:        dbAppt.LastName,
			MobilePhone:     dbAppt.MobilePhone,
			AppointmentType: dbAppt.AppointmentType,
			ScheduledDate:   *dbAppt.ScheduledDate,
			ScheduledTime:   dbAppt.ScheduledTime,
		})
	}
	c.JSON(http.StatusOK, appointments)
}

func (s *Server) handlerGetAppointmentById(c *gin.Context) {
	id, err := parseId(c, "id")
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}

	dbAppt, err := s.DBQuery.GetAppointmentById(c, id)
	if err != nil || err == sql.ErrNoRows {
		respondWithError(c, http.StatusNotFound, "No appointments found", err)
		return
	}
	c.JSON(http.StatusOK, Appointment{
		ID:              dbAppt.ID,
		CreatedAt:       dbAppt.CreatedAt,
		ModifiedAt:      dbAppt.ModifiedAt,
		Email:           dbAppt.Email,
		FirstName:       dbAppt.FirstName,
		LastName:        dbAppt.LastName,
		MobilePhone:     dbAppt.MobilePhone,
		RequestedDate:   dbAppt.RequestedDate,
		IsEmergency:     dbAppt.IsEmergency,
		Description:     dbAppt.Description,
		AppointmentType: &dbAppt.AppointmentType,
		IsScheduled:     &dbAppt.IsScheduled,
		ScheduledDate:   dbAppt.ScheduledDate,
		CreatedBy:       dbAppt.CreatedBy,
		ScheduledBy:     dbAppt.ScheduledBy,
		IsCancelled:     &dbAppt.IsCancelled,
		RequestedTime:   dbAppt.RequestedTime,
		ScheduledTime:   dbAppt.ScheduledTime,
		PracticeID:      dbAppt.PracticeID,
		Token:           dbAppt.Token,
	})
}

func (s *Server) handlerAppointmentsCreate(c *gin.Context) {
	decoder := json.NewDecoder(c.Request.Body)
	params := NewAppointmentRequest{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	apptToken := MakeToken()

	requestedDate, err := time.Parse("2006-01-02", params.RequestedDate)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid date format, expected YYYY-MM-DD", err)
		return
	}

	appointment, err := s.DBQuery.CreateAppointment(c, db.CreateAppointmentParams{
		FirstName:       params.FirstName,
		LastName:        params.LastName,
		MobilePhone:     params.MobilePhone,
		Email:           params.Email,
		RequestedDate:   &requestedDate,
		RequestedTime:   params.RequestedTime,
		AppointmentType: &params.AppointmentType,
		Description:     &params.Description,
		IsEmergency:     params.IsEmergency,
		PracticeID:      &params.PracticeID,
		Token:           apptToken,
	})

	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not save to database", err)
		return
	}
	c.JSON(http.StatusCreated, Appointment{
		ID:              appointment.ID,
		CreatedAt:       appointment.CreatedAt,
		ModifiedAt:      appointment.ModifiedAt,
		FirstName:       appointment.FirstName,
		LastName:        appointment.LastName,
		MobilePhone:     appointment.MobilePhone,
		Email:           appointment.Email,
		RequestedDate:   appointment.RequestedDate,
		RequestedTime:   appointment.RequestedTime,
		AppointmentType: &appointment.AppointmentType,
		Description:     appointment.Description,
		IsEmergency:     appointment.IsEmergency,
		PracticeID:      appointment.PracticeID,
		Token:           appointment.Token,
	})
}

func (s *Server) handlerAppointmentsUpdate(c *gin.Context) {
	id, err := parseId(c, "id")
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}

	var req UpdateAppointmentRequest

	if err = c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "Could not decode request", err)
		return
	}
	updatedAppt, err := s.DBQuery.UpdateAppointment(c, db.UpdateAppointmentParams{
		ID:            id,
		ScheduledDate: req.ScheduledDate,
		ScheduledTime: req.ScheduledTime,
		IsScheduled:   &req.IsScheduled,
		IsCancelled:   &req.IsCancelled,
	})

	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not update appointment", err)
		return
	}

	c.JSON(http.StatusOK, Appointment{
		ID:              updatedAppt.ID,
		CreatedAt:       updatedAppt.CreatedAt,
		ModifiedAt:      updatedAppt.ModifiedAt,
		FirstName:       updatedAppt.FirstName,
		LastName:        updatedAppt.LastName,
		MobilePhone:     updatedAppt.MobilePhone,
		Email:           updatedAppt.Email,
		RequestedDate:   updatedAppt.RequestedDate,
		RequestedTime:   updatedAppt.RequestedTime,
		AppointmentType: &updatedAppt.AppointmentType,
		Description:     updatedAppt.Description,
		IsEmergency:     updatedAppt.IsEmergency,
		PracticeID:      updatedAppt.PracticeID,
		IsScheduled:     &updatedAppt.IsScheduled,
		IsCancelled:     &updatedAppt.IsCancelled,
		ScheduledDate:   updatedAppt.ScheduledDate,
		ScheduledTime:   updatedAppt.ScheduledTime,
	})
}

func (s *Server) handlerAppointmentsDelete(c *gin.Context) {
	id, err := parseId(c, "id")
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}

	deletedAppt, err := s.DBQuery.DeleteAppointment(c, id)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not delete appointment", err)
		return
	}

	c.JSON(http.StatusOK, fmt.Sprintf("Successfully deleted appointment with id: %v", deletedAppt))
}

package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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
	Description     string     `json:"description,omitempty"`
	AppointmentType string     `json:"appointment_type"`
	IsScheduled     bool       `json:"is_scheduled"`
	ScheduledDate   *time.Time `json:"scheduled_date,omitempty"`
	ScheduledTime   *string    `json:"scheduled_time,omitempty"`
	CreatedBy       *uuid.UUID `json:"created_by,omitempty"`
	ScheduledBy     *uuid.UUID `json:"scheduled_by,omitempty"`
	IsCancelled     bool       `json:"is_cancelled"`
	RequestedTime   string     `json:"requested_time"`
	PracticeID      uuid.UUID  `json:"practice_id"`
	Token           string     `json:"token"`
}

type NewAppointmentRequest struct {
	Email           string    `json:"email"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	MobilePhone     string    `json:"mobile_phone"`
	RequestedDate   time.Time `json:"requested_date"`
	IsEmergency     bool      `json:"is_emergency"`
	Description     *string   `json:"description"`
	AppointmentType *string   `json:"appointment_type"`
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

func (s *Server) handlerAppointmentsGetAll(w http.ResponseWriter, r *http.Request) {
	dbAppts, err := s.db.GetAppointments(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not get Appointments", err)
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
			Description:     *dbAppt.Description,
			AppointmentType: *dbAppt.AppointmentType,
			IsScheduled:     dbAppt.IsScheduled,
			ScheduledDate:   dbAppt.ScheduledDate,
			CreatedBy:       dbAppt.CreatedBy,
			ScheduledBy:     dbAppt.ScheduledBy,
			IsCancelled:     dbAppt.IsCancelled,
			RequestedTime:   dbAppt.RequestedTime,
			ScheduledTime:   dbAppt.ScheduledTime,
			PracticeID:      dbAppt.PracticeID,
			Token:           dbAppt.Token,
		})
	}
	respondWithJSON(w, http.StatusOK, appointments)
}

func (s *Server) handlerGetAppointmentById(w http.ResponseWriter, r *http.Request) {
	id, err := parseId(r, "id")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}

	dbAppt, err := s.DB.GetAppointmentById(r.Context(), id)
	if err != nil || err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "No appointments found", err)
		return
	}
	respondWithJSON(w, http.StatusOK, Appointment{
		ID:              dbAppt.ID,
		CreatedAt:       dbAppt.CreatedAt,
		ModifiedAt:      dbAppt.ModifiedAt,
		Email:           dbAppt.Email,
		FirstName:       dbAppt.FirstName,
		LastName:        dbAppt.LastName,
		MobilePhone:     dbAppt.MobilePhone,
		RequestedDate:   dbAppt.RequestedDate,
		IsEmergency:     dbAppt.IsEmergency,
		Description:     *dbAppt.Description,
		AppointmentType: *dbAppt.AppointmentType,
		IsScheduled:     dbAppt.IsScheduled,
		ScheduledDate:   dbAppt.ScheduledDate,
		CreatedBy:       dbAppt.CreatedBy,
		ScheduledBy:     dbAppt.ScheduledBy,
		IsCancelled:     dbAppt.IsCancelled,
		RequestedTime:   dbAppt.RequestedTime,
		ScheduledTime:   dbAppt.ScheduledTime,
		PracticeID:      dbAppt.PracticeID,
		Token:           dbAppt.Token,
	})
}

func (s *Server) handlerAppointmentsCreate(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	params := NewAppointmentRequest{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	apptToken := auth.MakeToken()
	fmt.Println(apptToken)

	appointment, err := s.DB.CreateAppointment(r.Context(), db.CreateAppointmentParams{
		FirstName:       params.FirstName,
		LastName:        params.LastName,
		MobilePhone:     params.MobilePhone,
		Email:           params.Email,
		RequestedDate:   params.RequestedDate,
		RequestedTime:   params.RequestedTime,
		AppointmentType: params.AppointmentType,
		Description:     params.Description,
		IsEmergency:     params.IsEmergency,
		PracticeID:      params.PracticeID,
		Token:           apptToken,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not save to database", err)
		return
	}
	respondWithJSON(w, http.StatusCreated, Appointment{
		ID:              appointment.ID,
		CreatedAt:       appointment.CreatedAt,
		ModifiedAt:      appointment.ModifiedAt,
		FirstName:       appointment.FirstName,
		LastName:        appointment.LastName,
		MobilePhone:     appointment.MobilePhone,
		Email:           appointment.Email,
		RequestedDate:   appointment.RequestedDate,
		RequestedTime:   appointment.RequestedTime,
		AppointmentType: *appointment.AppointmentType,
		Description:     *appointment.Description,
		IsEmergency:     appointment.IsEmergency,
		PracticeID:      appointment.PracticeID,
		Token:           appointment.Token,
	})
}

func (s *Server) handlerAppointmentsUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseId(r, "id")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}
	decoder := json.NewDecoder(r.Body)
	params := UpdateAppointmentRequest{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not decode request", err)
		return
	}
	updatedAppt, err := s.DB.UpdateAppointment(r.Context(), db.UpdateAppointmentParams{
		ID:            id,
		ScheduledDate: params.ScheduledDate,
		ScheduledTime: params.ScheduledTime,
		IsScheduled:   &params.IsScheduled,
		IsCancelled:   &params.IsCancelled,
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not update appointment", err)
		return
	}

	respondWithJSON(w, http.StatusOK, Appointment{
		ID:              updatedAppt.ID,
		CreatedAt:       updatedAppt.CreatedAt,
		ModifiedAt:      updatedAppt.ModifiedAt,
		FirstName:       updatedAppt.FirstName,
		LastName:        updatedAppt.LastName,
		MobilePhone:     updatedAppt.MobilePhone,
		Email:           updatedAppt.Email,
		RequestedDate:   updatedAppt.RequestedDate,
		RequestedTime:   updatedAppt.RequestedTime,
		AppointmentType: *updatedAppt.AppointmentType,
		Description:     *updatedAppt.Description,
		IsEmergency:     updatedAppt.IsEmergency,
		PracticeID:      updatedAppt.PracticeID,
		IsScheduled:     updatedAppt.IsScheduled,
		IsCancelled:     updatedAppt.IsCancelled,
		ScheduledDate:   updatedAppt.ScheduledDate,
		ScheduledTime:   updatedAppt.ScheduledTime,
	})
}

func (s *Server) handlerAppointmentsDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseId(r, "id")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid or missing ID", err)
		return
	}

	deletedAppt, err := s.DB.DeleteAppointment(r.Context(), id)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not delete appointment", err)
		return
	}

	respondWithJSON(w, http.StatusOK, fmt.Sprintf("Successfully deleted appointment with id: %v", deletedAppt))
}

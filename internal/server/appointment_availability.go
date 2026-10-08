package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const (
	availabilityDayStartMinutes = 7*60 + 30
	availabilityDayEndMinutes   = 17 * 60
	availabilityCadenceMinutes  = 15
)

type availabilityConflict struct {
	ID              uuid.UUID `json:"id"`
	PatientName     string    `json:"patientName"`
	AppointmentType *string   `json:"appointmentType"`
	Start           string    `json:"start"`
	End             string    `json:"end"`
}

type availabilitySlot struct {
	Start     string                 `json:"start"`
	End       string                 `json:"end"`
	Status    string                 `json:"status"`
	Conflicts []availabilityConflict `json:"conflicts"`
}

type availabilityDay struct {
	Date  string             `json:"date"`
	Slots []availabilitySlot `json:"slots"`
}

type availabilityQuery struct {
	Start                time.Time
	Days                 int
	ProviderID           uuid.UUID
	DurationMinutes      int
	ExcludeAppointmentID *uuid.UUID
}

func (s *Server) handlerGetAppointmentAvailability(c *gin.Context) {
	setPrivateNoStore(c)
	query, err := parseAvailabilityQuery(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid availability query"})
		return
	}
	user := c.MustGet("user").(AuthUser)
	if _, err := s.DBQuery.GetPracticeProviderLink(c, db.GetPracticeProviderLinkParams{
		PracticeID: *user.PracticeId, ProviderID: query.ProviderID,
	}); errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider not found"})
		return
	} else if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to verify provider", err)
		return
	}
	if query.ExcludeAppointmentID != nil {
		if _, err := s.DBQuery.GetPracticeAppointmentID(c, db.GetPracticeAppointmentIDParams{
			ID: *query.ExcludeAppointmentID, PracticeID: *user.PracticeId,
		}); errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found"})
			return
		} else if err != nil {
			respondWithError(c, http.StatusInternalServerError, "Failed to verify appointment", err)
			return
		}
	}
	end := query.Start.AddDate(0, 0, query.Days-1)
	providerID := query.ProviderID
	busy, err := s.DBQuery.ListProviderBusyAppointments(c, db.ListProviderBusyAppointmentsParams{
		PracticeID: *user.PracticeId, ProviderID: &providerID, StartDate: &query.Start,
		EndDate: &end, ExcludeAppointmentID: query.ExcludeAppointmentID,
	})
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to fetch availability", err)
		return
	}
	c.JSON(http.StatusOK, generateAvailability(query, busy))
}

func parseAvailabilityQuery(c *gin.Context) (availabilityQuery, error) {
	allowed := map[string]bool{
		"start": true, "days": true, "providerId": true,
		"durationMinutes": true, "excludeAppointmentId": true,
	}
	for key := range c.Request.URL.Query() {
		if !allowed[key] {
			return availabilityQuery{}, fmt.Errorf("unknown query field")
		}
	}
	start, err := time.Parse("2006-01-02", c.Query("start"))
	if err != nil {
		return availabilityQuery{}, err
	}
	days, err := strconv.Atoi(c.Query("days"))
	if err != nil || days < 1 || days > 7 {
		return availabilityQuery{}, fmt.Errorf("invalid days")
	}
	providerID, err := uuid.Parse(c.Query("providerId"))
	if err != nil {
		return availabilityQuery{}, err
	}
	duration, err := strconv.Atoi(c.Query("durationMinutes"))
	if err != nil || duration < 15 || duration > 120 || duration%15 != 0 {
		return availabilityQuery{}, fmt.Errorf("invalid duration")
	}
	var excludeID *uuid.UUID
	if value, ok := c.GetQuery("excludeAppointmentId"); ok {
		id, err := uuid.Parse(value)
		if err != nil {
			return availabilityQuery{}, err
		}
		excludeID = &id
	}
	return availabilityQuery{
		Start: start, Days: days, ProviderID: providerID,
		DurationMinutes: duration, ExcludeAppointmentID: excludeID,
	}, nil
}

func generateAvailability(query availabilityQuery, busy []db.ListProviderBusyAppointmentsRow) []availabilityDay {
	days := make([]availabilityDay, 0, query.Days)
	for offset := 0; offset < query.Days; offset++ {
		date := query.Start.AddDate(0, 0, offset).Format("2006-01-02")
		slots := make([]availabilitySlot, 0)
		for start := availabilityDayStartMinutes; start+query.DurationMinutes <= availabilityDayEndMinutes; start += availabilityCadenceMinutes {
			end := start + query.DurationMinutes
			conflicts := make([]availabilityConflict, 0)
			for _, appointment := range busy {
				if appointment.ScheduledDate == nil || appointment.ScheduledTime == nil || appointment.ScheduledDate.Format("2006-01-02") != date {
					continue
				}
				appointmentStart, ok := parseClockMinutes(*appointment.ScheduledTime)
				if !ok {
					continue
				}
				appointmentDuration := 15
				if appointment.DurationMinutes != nil {
					appointmentDuration = int(*appointment.DurationMinutes)
				}
				appointmentEnd := appointmentStart + appointmentDuration
				if start < appointmentEnd && end > appointmentStart {
					conflicts = append(conflicts, availabilityConflict{
						ID: appointment.ID, PatientName: appointment.FirstName + " " + appointment.LastName,
						AppointmentType: appointment.AppointmentType,
						Start:           localAppointmentDateTime(date, appointmentStart),
						End:             localAppointmentDateTime(date, appointmentEnd),
					})
				}
			}
			status := "available"
			if len(conflicts) > 0 {
				status = "conflict"
			}
			slots = append(slots, availabilitySlot{
				Start: localAppointmentDateTime(date, start), End: localAppointmentDateTime(date, end),
				Status: status, Conflicts: conflicts,
			})
		}
		days = append(days, availabilityDay{Date: date, Slots: slots})
	}
	return days
}

func parseClockMinutes(value string) (int, bool) {
	parsed, err := time.Parse("15:04:05", value)
	if err != nil {
		parsed, err = time.Parse("15:04", value)
	}
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

func localAppointmentDateTime(date string, minutes int) string {
	return fmt.Sprintf("%sT%02d:%02d:00", date, minutes/60, minutes%60)
}

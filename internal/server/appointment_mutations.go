package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type staffAppointmentTransactionResult struct {
	Appointment db.Appointment
	Patient     patientSummary
	Conflicts   []availabilityConflict
}

type confirmAppointmentInput struct {
	SendEmail    bool
	SendWhatsApp bool
	NotifyEmail  *string
	NotifyPhone  *string
}

func (s *Server) handlerCreateStaffAppointment(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	active, err := s.practiceHasActiveSubscription(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to verify subscription", err)
		return
	}
	if !active {
		c.JSON(http.StatusForbidden, gin.H{"error": "An active subscription is required"})
		return
	}
	input, err := decodeCreateStaffAppointment(c.Request.Body)
	if err != nil {
		if errors.Is(err, errCompleteMobileNumberRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": errCompleteMobileNumberRequired.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment details"})
		return
	}
	result, err := s.createStaffAppointmentTransaction(c, user, input)
	if err != nil {
		var duplicate *existingPatientError
		if errors.As(err, &duplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": "patient_exists", "patient": duplicate.Patient})
			return
		}
		status, message := schedulingErrorResponse(err, "Failed to create appointment")
		if status >= 500 {
			log.Println(err)
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	if len(result.Conflicts) > 0 {
		c.JSON(http.StatusConflict, gin.H{"scheduled": false, "appointment": nil, "conflicts": result.Conflicts})
		return
	}
	s.runAppointmentCreatedEffects(c, result.Appointment, result.Patient, input)
	notifications := gin.H{"email": "not_requested", "whatsapp": "not_requested"}
	if input.IsConfirmed && input.SendEmail && result.Patient.Email != nil {
		notifications["email"] = s.sendAppointmentEmail(c, appointmentNotification(result.Appointment))
	}
	if input.IsConfirmed && input.SendWhatsApp && result.Patient.MobilePhone != nil {
		notifications["whatsapp"] = s.sendAppointmentWhatsApp(c, appointmentNotification(result.Appointment))
	}
	c.JSON(http.StatusOK, gin.H{
		"scheduled": true, "appointment": appointmentResponse(result.Appointment),
		"conflicts": []availabilityConflict{}, "notifications": notifications,
	})
}

func (s *Server) handlerScheduleAppointment(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}
	input, err := decodeScheduleAppointment(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid scheduling request"})
		return
	}
	result, err := s.scheduleAppointmentTransaction(c, user, id, input)
	if err != nil {
		status, message := schedulingErrorResponse(err, "Failed to schedule appointment")
		if status >= 500 {
			log.Println(err)
		}
		c.JSON(status, gin.H{"error": message})
		return
	}
	if !result.Scheduled {
		c.JSON(http.StatusConflict, gin.H{"scheduled": false, "appointment": nil, "conflicts": result.Conflicts})
		return
	}
	s.runAppointmentScheduledEffects(c, result, input)
	if input.IsConfirmed && input.SendEmail != nil && *input.SendEmail {
		notification := appointmentNotification(result.Appointment)
		if input.NotifyEmail != nil {
			notification.Email = *input.NotifyEmail
		}
		_ = s.sendAppointmentEmail(c, notification)
	}
	if input.IsConfirmed && (input.SendWhatsApp == nil || *input.SendWhatsApp) {
		phone := result.Previous.MobilePhone
		if input.NotifyPhone != nil && *input.NotifyPhone != "" {
			phone = *input.NotifyPhone
		}
		if phone != "" {
			notification := appointmentNotification(result.Appointment)
			notification.MobilePhone = phone
			_ = s.sendAppointmentWhatsApp(c, notification)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"scheduled": true, "appointment": appointmentResponse(result.Appointment),
		"conflicts": []availabilityConflict{},
	})
}

func (s *Server) handlerConfirmAppointment(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}
	input, err := decodeConfirmAppointment(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid confirmation request"})
		return
	}
	params := db.ConfirmAppointmentParams{ID: id, PracticeID: *user.PracticeId}
	if input.NotifyEmail != nil {
		params.SetEmail, params.Email = true, *input.NotifyEmail
	}
	if input.NotifyPhone != nil {
		params.SetMobilePhone, params.MobilePhone = true, *input.NotifyPhone
	}
	appointment, err := s.DBQuery.ConfirmAppointment(c, params)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusConflict, gin.H{"error": "Appointment cannot be confirmed"})
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to confirm appointment", err)
		return
	}
	s.broadcastAppointmentChange(c, appointment.PracticeID, "UPDATE", appointment.ID)
	notifications := gin.H{}
	if input.SendEmail {
		notifications["email"] = s.sendAppointmentEmail(c, appointmentNotification(appointment))
	}
	if input.SendWhatsApp {
		notifications["whatsapp"] = s.sendAppointmentWhatsApp(c, appointmentNotification(appointment))
	}
	c.JSON(http.StatusOK, gin.H{"appointment": appointmentResponse(appointment), "notifications": notifications})
}

func (s *Server) handlerCancelAppointment(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid appointment ID"})
		return
	}
	if err := decodeEmptyJSONObject(c.Request.Body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid cancellation request"})
		return
	}
	appointment, changed, err := s.cancelAppointmentTransaction(c, *user.PracticeId, id)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Appointment not found"})
		return
	}
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Failed to cancel appointment", err)
		return
	}
	if changed {
		s.broadcastAppointmentChange(c, appointment.PracticeID, "DELETE", appointment.ID)
		if s.appointmentEvents != nil {
			if err := s.appointmentEvents.SyncAppointmentCancelled(c, appointment.PracticeID, appointment.ID); err != nil {
				log.Printf("appointment calendar cancellation failed for %s: %v", appointment.ID, err)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"appointment": appointmentResponse(appointment)})
}

func decodeEmptyJSONObject(body io.Reader) error {
	var fields map[string]json.RawMessage
	if err := decodeStrictJSON(body, &fields); err != nil || fields == nil || len(fields) != 0 {
		return errors.New("expected empty object")
	}
	return nil
}

func (s *Server) cancelAppointmentTransaction(ctx context.Context, practiceID, id uuid.UUID) (db.Appointment, bool, error) {
	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return db.Appointment{}, false, err
	}
	defer tx.Rollback(ctx)
	queries := s.DBQuery.WithTx(tx)
	appointment, err := queries.LockAppointmentForCancellation(ctx, db.LockAppointmentForCancellationParams{ID: id, PracticeID: practiceID})
	if err != nil {
		return db.Appointment{}, false, err
	}
	if appointment.IsCancelled {
		if err := tx.Commit(ctx); err != nil {
			return db.Appointment{}, false, err
		}
		return appointment, false, nil
	}
	appointment, err = queries.CancelAppointment(ctx, db.CancelAppointmentParams{ID: id, PracticeID: practiceID})
	if err != nil {
		return db.Appointment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Appointment{}, false, err
	}
	return appointment, true, nil
}

func (s *Server) createStaffAppointmentTransaction(ctx context.Context, user AuthUser, input createStaffAppointmentInput) (staffAppointmentTransactionResult, error) {
	date, err := validateScheduleValues(input.ScheduledDate, input.ScheduledTime, input.DurationMinutes)
	if err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAppointmentScheduleNamespace(ctx, tx, input.ProviderID, input.ScheduledDate); err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	queries := s.DBQuery.WithTx(tx)
	if err := validateSchedulingResources(ctx, queries, *user.PracticeId, input.ProviderID, input.LocationID, date); err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	providerID := input.ProviderID
	duration := int32(input.DurationMinutes)
	conflictRows, err := queries.ListSchedulingConflicts(ctx, db.ListSchedulingConflictsParams{
		PracticeID: *user.PracticeId, ProviderID: &providerID, ScheduledDate: date,
		ScheduledTime: input.ScheduledTime, DurationMinutes: duration,
	})
	if err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	conflicts := scheduleConflicts(conflictRows)
	if unacknowledgedConflicts(conflicts, input.AcknowledgedConflictIDs) {
		return staffAppointmentTransactionResult{Conflicts: conflicts}, nil
	}
	patient, err := createOrUpdateAppointmentPatient(ctx, tx, queries, *user.PracticeId, input.Patient)
	if err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	appointmentType := input.AppointmentType
	email, phone := pointerString(patient.Email), pointerString(patient.MobilePhone)
	appointment, err := queries.CreateStaffAppointment(ctx, db.CreateStaffAppointmentParams{
		PracticeID: *user.PracticeId, PatientID: &patient.ID,
		FirstName: patient.FirstName, LastName: patient.LastName, Email: email, MobilePhone: phone,
		AppointmentType: &appointmentType, ProviderID: &providerID, LocationID: input.LocationID,
		DurationMinutes: &duration, ScheduledDate: date, ScheduledTime: input.ScheduledTime,
		IsConfirmed: input.IsConfirmed, CreatedBy: &user.ID, ScheduledBy: &user.ID,
		ScheduledTimezone: input.Timezone,
	})
	if err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return staffAppointmentTransactionResult{}, err
	}
	return staffAppointmentTransactionResult{Appointment: appointment, Patient: patient}, nil
}

func (s *Server) scheduleAppointmentTransaction(ctx context.Context, user AuthUser, id uuid.UUID, input scheduleAppointmentInput) (scheduleTransactionResult, error) {
	date, err := validateScheduleValues(input.ScheduledDate, input.ScheduledTime, input.DurationMinutes)
	if err != nil {
		return scheduleTransactionResult{}, err
	}
	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return scheduleTransactionResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAppointmentScheduleNamespace(ctx, tx, input.ProviderID, input.ScheduledDate); err != nil {
		return scheduleTransactionResult{}, err
	}
	queries := s.DBQuery.WithTx(tx)
	previous, err := queries.LockAppointmentForScheduling(ctx, db.LockAppointmentForSchedulingParams{ID: id, PracticeID: *user.PracticeId})
	if errors.Is(err, pgx.ErrNoRows) {
		return scheduleTransactionResult{}, &schedulingError{Status: 404, Message: "Appointment not found"}
	}
	if err != nil {
		return scheduleTransactionResult{}, err
	}
	if previous.IsCancelled {
		return scheduleTransactionResult{}, &schedulingError{Status: http.StatusConflict, Message: "Cancelled appointments cannot be scheduled"}
	}
	finalLocation := previous.LocationID
	if input.Location.Set {
		finalLocation = input.Location.Value
	}
	if err := validateSchedulingResources(ctx, queries, *user.PracticeId, input.ProviderID, finalLocation, date); err != nil {
		return scheduleTransactionResult{}, err
	}
	providerID := input.ProviderID
	duration := int32(input.DurationMinutes)
	conflictRows, err := queries.ListSchedulingConflicts(ctx, db.ListSchedulingConflictsParams{
		PracticeID: *user.PracticeId, ProviderID: &providerID, ExcludeAppointmentID: &id,
		ScheduledDate: date, ScheduledTime: input.ScheduledTime, DurationMinutes: duration,
	})
	if err != nil {
		return scheduleTransactionResult{}, err
	}
	conflicts := scheduleConflicts(conflictRows)
	if unacknowledgedConflicts(conflicts, input.AcknowledgedConflictIDs) {
		return scheduleTransactionResult{Previous: previous, Conflicts: conflicts}, nil
	}
	appointment, err := queries.ScheduleAppointment(ctx, db.ScheduleAppointmentParams{
		IsConfirmed: input.IsConfirmed, ScheduledDate: date, ScheduledTime: input.ScheduledTime,
		ProviderID: &providerID, SetLocation: input.Location.Set, LocationID: input.Location.Value,
		SetAppointmentType: input.AppointmentType.Set, AppointmentType: input.AppointmentType.Value,
		DurationMinutes: &duration, ScheduledBy: &user.ID, ID: id, PracticeID: *user.PracticeId,
		ScheduledTimezone: input.Timezone,
	})
	if err != nil {
		return scheduleTransactionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return scheduleTransactionResult{}, err
	}
	return scheduleTransactionResult{Previous: previous, Scheduled: true, Appointment: appointment}, nil
}

func createOrUpdateAppointmentPatient(ctx context.Context, tx pgx.Tx, queries *db.Queries, practiceID uuid.UUID, input staffAppointmentPatientInput) (patientSummary, error) {
	phone := normalizePatientPhone(input.MobilePhone)
	if phone == "" {
		return patientSummary{}, &schedulingError{Status: 400, Message: "A complete mobile number is required"}
	}
	email := input.Email
	if input.Kind == "existing" {
		row, err := queries.LockPatientForStaffAppointment(ctx, db.LockPatientForStaffAppointmentParams{ID: input.ID, PracticeID: practiceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return patientSummary{}, &schedulingError{Status: 404, Message: "Patient not found"}
		}
		if err != nil {
			return patientSummary{}, err
		}
		if email == nil {
			email = row.Email
		}
		if email != nil {
			normalized := strings.ToLower(strings.TrimSpace(*email))
			email = &normalized
		}
		if err := lockPatientContacts(ctx, tx, practiceID, pointerString(email), phone); err != nil {
			return patientSummary{}, err
		}
		if _, err := queries.FindOtherPatientByContact(ctx, db.FindOtherPatientByContactParams{
			PracticeID: practiceID, ID: row.ID, Email: email, MobilePhone: &phone,
		}); err == nil {
			return patientSummary{}, &schedulingError{Status: 400, Message: "That contact is already used by another patient"}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return patientSummary{}, err
		}
		updated, err := queries.UpdateStaffAppointmentPatient(ctx, db.UpdateStaffAppointmentPatientParams{
			Email: email, MobilePhone: &phone, ID: row.ID, PracticeID: practiceID,
		})
		if err != nil {
			return patientSummary{}, err
		}
		return patientSummary{ID: updated.ID, FirstName: updated.FirstName, LastName: updated.LastName, Email: updated.Email, MobilePhone: updated.MobilePhone}, nil
	}
	if err := lockPatientContacts(ctx, tx, practiceID, pointerString(email), phone); err != nil {
		return patientSummary{}, err
	}
	existing, err := queries.FindPatientByContact(ctx, db.FindPatientByContactParams{PracticeID: practiceID, Email: email, MobilePhone: phone})
	if err == nil {
		return patientSummary{}, &existingPatientError{Patient: patientSummary{
			ID: existing.ID, FirstName: existing.FirstName, LastName: existing.LastName,
			Email: existing.Email, MobilePhone: existing.MobilePhone,
		}}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return patientSummary{}, err
	}
	created, err := queries.CreateStaffAppointmentPatient(ctx, db.CreateStaffAppointmentPatientParams{
		PracticeID: practiceID, FirstName: input.FirstName, LastName: input.LastName,
		Email: email, MobilePhone: &phone,
	})
	if err != nil {
		return patientSummary{}, err
	}
	return patientSummary{ID: created.ID, FirstName: created.FirstName, LastName: created.LastName, Email: created.Email, MobilePhone: created.MobilePhone}, nil
}

func decodeConfirmAppointment(body io.Reader) (confirmAppointmentInput, error) {
	var wire struct {
		SendEmail    optionalBoolInput   `json:"sendEmail"`
		SendWhatsApp optionalBoolInput   `json:"sendWhatsApp"`
		NotifyEmail  optionalStringInput `json:"notifyEmail"`
		NotifyPhone  optionalStringInput `json:"notifyPhone"`
	}
	if err := decodeStrictJSON(body, &wire); err != nil {
		return confirmAppointmentInput{}, err
	}
	email, err := validateOptionalEmail(optionalStringPointer(wire.NotifyEmail))
	if err != nil {
		return confirmAppointmentInput{}, err
	}
	var phone *string
	if wire.NotifyPhone.Set {
		value, err := validatePhone(wire.NotifyPhone.Value)
		if err != nil {
			return confirmAppointmentInput{}, err
		}
		phone = &value
	}
	if wire.SendEmail.Value && email == nil {
		return confirmAppointmentInput{}, errors.New("email required")
	}
	if wire.SendWhatsApp.Value && phone == nil {
		return confirmAppointmentInput{}, errors.New("phone required")
	}
	return confirmAppointmentInput{
		SendEmail: wire.SendEmail.Value, SendWhatsApp: wire.SendWhatsApp.Value,
		NotifyEmail: email, NotifyPhone: phone,
	}, nil
}

func (s *Server) practiceHasActiveSubscription(ctx context.Context, practiceID uuid.UUID) (bool, error) {
	row, err := s.DBQuery.GetPracticeSubscription(ctx, practiceID)
	if err != nil {
		return false, err
	}
	status := computeSubscriptionContext(
		row.Status, row.Plan, nullableTimestamp(row.TrialEnd), nullableTimestamp(row.PeriodEnd),
		nullableTimestamp(row.CancelAt), time.Now(),
	)
	return status.IsActive, nil
}

func appointmentNotification(appointment db.Appointment) AppointmentNotification {
	return AppointmentNotification{
		AppointmentID: appointment.ID, PracticeID: appointment.PracticeID,
		FirstName: appointment.FirstName, LastName: appointment.LastName,
		Email: appointment.Email, MobilePhone: appointment.MobilePhone,
		AppointmentType: pointerString(appointment.AppointmentType),
		ScheduledDate:   pointerString(dateString(appointment.ScheduledDate)),
		ScheduledTime:   pointerString(appointment.ScheduledTime),
		ProviderID:      appointment.ProviderID, LocationID: appointment.LocationID,
	}
}

func (s *Server) sendAppointmentEmail(ctx context.Context, notification AppointmentNotification) RegistrationDeliveryResult {
	if s.appointmentNotify == nil {
		return RegistrationDeliveryUnavailable
	}
	result, err := s.appointmentNotify.SendAppointmentEmail(ctx, notification)
	if err != nil {
		log.Printf("appointment email failed for %s: %v", notification.AppointmentID, err)
		if result == "" {
			return RegistrationDeliveryFailed
		}
	}
	return result
}

func (s *Server) sendAppointmentWhatsApp(ctx context.Context, notification AppointmentNotification) RegistrationDeliveryResult {
	if s.appointmentNotify == nil {
		return RegistrationDeliveryUnavailable
	}
	result, err := s.appointmentNotify.SendAppointmentWhatsApp(ctx, notification)
	if err != nil {
		log.Printf("appointment WhatsApp failed for %s: %v", notification.AppointmentID, err)
		if result == "" {
			return RegistrationDeliveryFailed
		}
	}
	return result
}

func (s *Server) runAppointmentCreatedEffects(ctx context.Context, appointment db.Appointment, patient patientSummary, input createStaffAppointmentInput) {
	s.broadcastAppointmentChange(ctx, appointment.PracticeID, "INSERT", appointment.ID)
	if s.appointmentEvents == nil {
		return
	}
	event := AppointmentEvent{
		AppointmentID: appointment.ID, PracticeID: appointment.PracticeID,
		PatientName:   strings.TrimSpace(patient.FirstName + " " + patient.LastName),
		ProcedureType: input.AppointmentType, Phone: pointerString(patient.MobilePhone),
		ScheduledDate: input.ScheduledDate, ScheduledTime: input.ScheduledTime,
		Timezone: input.Timezone, DurationMinutes: input.DurationMinutes,
	}
	if err := s.appointmentEvents.SyncAppointmentCreated(ctx, event); err != nil {
		log.Printf("appointment calendar create failed for %s: %v", appointment.ID, err)
	}
}

func (s *Server) runAppointmentScheduledEffects(ctx context.Context, result scheduleTransactionResult, input scheduleAppointmentInput) {
	s.broadcastAppointmentChange(ctx, result.Appointment.PracticeID, "UPDATE", result.Appointment.ID)
	if s.appointmentEvents == nil {
		return
	}
	event := AppointmentEvent{
		AppointmentID: result.Appointment.ID, PracticeID: result.Appointment.PracticeID,
		PatientName:   strings.TrimSpace(result.Previous.FirstName + " " + result.Previous.LastName),
		ProcedureType: pointerString(result.Appointment.AppointmentType), Phone: result.Previous.MobilePhone,
		ScheduledDate: input.ScheduledDate, ScheduledTime: input.ScheduledTime,
		Timezone: input.Timezone, DurationMinutes: input.DurationMinutes,
	}
	var err error
	if result.Previous.IsScheduled {
		err = s.appointmentEvents.SyncAppointmentUpdated(ctx, event)
	} else {
		err = s.appointmentEvents.SyncAppointmentCreated(ctx, event)
	}
	if err != nil {
		log.Printf("appointment calendar sync failed for %s: %v", result.Appointment.ID, err)
	}
}

func (s *Server) broadcastAppointmentChange(ctx context.Context, practiceID uuid.UUID, event string, appointmentID uuid.UUID) {
	if s.appointmentEvents == nil {
		return
	}
	if err := s.appointmentEvents.BroadcastAppointmentChange(ctx, practiceID, event, appointmentID); err != nil {
		log.Printf("appointment broadcast failed for %s: %v", appointmentID, err)
	}
}

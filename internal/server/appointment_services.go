package server

import (
	"context"

	"github.com/google/uuid"
)

type AppointmentNotification struct {
	AppointmentID   uuid.UUID
	PracticeID      uuid.UUID
	FirstName       string
	LastName        string
	Email           string
	MobilePhone     string
	AppointmentType string
	ScheduledDate   string
	ScheduledTime   string
	ProviderID      *uuid.UUID
	LocationID      *uuid.UUID
}

type AppointmentNotifier interface {
	SendAppointmentEmail(context.Context, AppointmentNotification) (RegistrationDeliveryResult, error)
	SendAppointmentWhatsApp(context.Context, AppointmentNotification) (RegistrationDeliveryResult, error)
}

type AppointmentEvent struct {
	AppointmentID   uuid.UUID
	PracticeID      uuid.UUID
	PatientName     string
	ProcedureType   string
	Phone           string
	ScheduledDate   string
	ScheduledTime   string
	Timezone        string
	DurationMinutes int
}

type AppointmentEventService interface {
	BroadcastAppointmentChange(context.Context, uuid.UUID, string, uuid.UUID) error
	SyncAppointmentCreated(context.Context, AppointmentEvent) error
	SyncAppointmentUpdated(context.Context, AppointmentEvent) error
	SyncAppointmentCancelled(context.Context, uuid.UUID, uuid.UUID) error
}

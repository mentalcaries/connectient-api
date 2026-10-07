package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestGenerateAvailability(t *testing.T) {
	start, _ := time.Parse("2006-01-02", "2026-10-08")
	duration := int32(30)
	appointmentDate := start
	appointmentTime := "08:00:00"
	typeName := "Consultation"
	days := generateAvailability(availabilityQuery{Start: start, Days: 1, DurationMinutes: 30}, []db.ListProviderBusyAppointmentsRow{{
		ID: uuid.New(), FirstName: "Jane", LastName: "Doe", AppointmentType: &typeName,
		ScheduledDate: &appointmentDate, ScheduledTime: &appointmentTime, DurationMinutes: &duration,
	}})
	if len(days) != 1 || len(days[0].Slots) != 37 {
		t.Fatalf("unexpected availability size: %+v", days)
	}
	for _, index := range []int{1, 2} { // 07:45-08:15 and 08:00-08:30 overlap.
		if days[0].Slots[index].Status != "conflict" || len(days[0].Slots[index].Conflicts) != 1 {
			t.Errorf("slot %d should conflict: %+v", index, days[0].Slots[index])
		}
	}
	if days[0].Slots[0].Status != "available" || days[0].Slots[4].Status != "available" {
		t.Errorf("adjacent slots should be available")
	}
}

func TestDecodeAppointmentSchedulingCommands(t *testing.T) {
	providerID := uuid.NewString()
	create, err := decodeCreateStaffAppointment(strings.NewReader(`{
		"patient":{"kind":"new","firstName":" Jane ","lastName":" Doe ","mobilePhone":" +1 (555) 123-4567 "},
		"appointmentType":" Consultation ","scheduledDate":"2026-10-08","scheduledTime":"08:00",
		"durationMinutes":30,"providerId":"` + providerID + `"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if create.Patient.FirstName != "Jane" || create.AppointmentType != "Consultation" || !create.SendEmail || !create.SendWhatsApp || create.Timezone != "UTC" {
		t.Errorf("unexpected create command: %+v", create)
	}

	for _, body := range []string{
		`{"patient":{"kind":"new","firstName":"A","lastName":"B","mobilePhone":"1234567"},"appointmentType":"X","scheduledDate":"2026-10-08","scheduledTime":"07:31","durationMinutes":15,"providerId":"` + providerID + `"}`,
		`{"patient":{"kind":"new","firstName":"A","lastName":"B","mobilePhone":"1234567","extra":true},"appointmentType":"X","scheduledDate":"2026-10-08","scheduledTime":"08:00","durationMinutes":15,"providerId":"` + providerID + `"}`,
		`{"patient":{"kind":"new","firstName":"A","lastName":"B","mobilePhone":"1234567"},"appointmentType":"X","scheduledDate":"2026-02-30","scheduledTime":"08:00","durationMinutes":15,"providerId":"` + providerID + `"}`,
	} {
		if _, err := decodeCreateStaffAppointment(strings.NewReader(body)); err == nil {
			t.Errorf("expected invalid command: %s", body)
		}
	}

	schedule, err := decodeScheduleAppointment(strings.NewReader(`{
		"scheduledDate":"2026-10-08","scheduledTime":"08:00","locationId":null,
		"appointmentType":null,"durationMinutes":15,"providerId":"` + providerID + `"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !schedule.Location.Set || schedule.Location.Value != nil || !schedule.AppointmentType.Set || schedule.AppointmentType.Value != nil {
		t.Errorf("nullable update state lost: %+v", schedule)
	}
}

func TestDecodeConfirmAppointment(t *testing.T) {
	if _, err := decodeConfirmAppointment(strings.NewReader(`{"sendEmail":true}`)); err == nil {
		t.Fatal("expected requested email without address to fail")
	}
	input, err := decodeConfirmAppointment(strings.NewReader(`{"sendEmail":false,"notifyEmail":" PATIENT@EXAMPLE.TEST "}`))
	if err != nil || input.NotifyEmail == nil || *input.NotifyEmail != "patient@example.test" {
		t.Fatalf("unexpected confirmation input: %+v err=%v", input, err)
	}
}

func TestAppointmentNotifierUnavailable(t *testing.T) {
	s := &Server{}
	notification := AppointmentNotification{AppointmentID: uuid.New()}
	if got := s.sendAppointmentEmail(context.Background(), notification); got != RegistrationDeliveryUnavailable {
		t.Errorf("email result = %q", got)
	}
	if got := s.sendAppointmentWhatsApp(context.Background(), notification); got != RegistrationDeliveryUnavailable {
		t.Errorf("WhatsApp result = %q", got)
	}
}

func TestDecodeEmptyCancellationBody(t *testing.T) {
	if err := decodeEmptyJSONObject(strings.NewReader(`{}`)); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", `null`, `[]`, `{"reason":"changed"}`} {
		if err := decodeEmptyJSONObject(strings.NewReader(body)); err == nil {
			t.Errorf("expected invalid cancellation body: %s", body)
		}
	}
}

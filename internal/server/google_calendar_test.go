package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestGoogleEventPayloadUsesWallClockTimezone(t *testing.T) {
	payload := googleEventPayload(AppointmentEvent{
		PatientName: "Jane Doe", ProcedureType: "Consultation", Phone: "+15550000000",
		ScheduledDate: "2026-10-08", ScheduledTime: "09:30", Timezone: "America/Port_of_Spain",
		DurationMinutes: 45,
	})
	start := payload["start"].(map[string]string)
	end := payload["end"].(map[string]string)
	if start["dateTime"] != "2026-10-08T09:30:00" || start["timeZone"] != "America/Port_of_Spain" ||
		end["dateTime"] != "2026-10-08T10:15:00" || end["timeZone"] != "America/Port_of_Spain" {
		t.Fatalf("unexpected event times: start=%v end=%v", start, end)
	}
}

func TestGoogleCalendarSafetyPreventsNetwork(t *testing.T) {
	calls := 0
	service := &googleCalendarService{
		env: func(key string) string {
			if key == "E2E_TEST_MODE" {
				return "true"
			}
			return ""
		},
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return testHTTPResponse(http.StatusOK), nil
		})},
	}
	event := AppointmentEvent{AppointmentID: uuid.New(), PracticeID: uuid.New()}
	if err := service.SyncAppointmentCreated(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := service.SyncAppointmentUpdated(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := service.SyncAppointmentCancelled(context.Background(), event.PracticeID, event.AppointmentID); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("calendar safety made %d network calls", calls)
	}
}

func TestGoogleEventURLNeverUsesPrimaryCalendar(t *testing.T) {
	url := googleEventURL("app-created-calendar@example.test", "event/one")
	if url == "" || url == googleEventURL("primary", "event/one") || url == "https://www.googleapis.com/calendar/v3/calendars/primary/events/event%2Fone" {
		t.Fatalf("unexpected event URL: %s", url)
	}
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type realtimeAppointmentService struct {
	client *http.Client
	env    func(string) string
}

func newRealtimeAppointmentService() *realtimeAppointmentService {
	return &realtimeAppointmentService{
		client: &http.Client{Timeout: 5 * time.Second},
		env:    os.Getenv,
	}
}

func (r *realtimeAppointmentService) BroadcastAppointmentChange(ctx context.Context, practiceID uuid.UUID, event string, appointmentID uuid.UUID) error {
	if r.env("E2E_TEST_MODE") == "true" || r.env("SKIP_REALTIME") == "true" {
		return nil
	}
	if event != "INSERT" && event != "UPDATE" && event != "DELETE" {
		return fmt.Errorf("invalid appointment broadcast event %q", event)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(r.env("SUPABASE_URL")), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(r.env("NEXT_PUBLIC_SUPABASE_URL")), "/")
	}
	apiKey := strings.TrimSpace(r.env("SUPABASE_SECRET_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(r.env("SUPABASE_SERVICE_ROLE_KEY"))
	}
	if baseURL == "" || apiKey == "" {
		return errors.New("Supabase Realtime is not configured")
	}
	payload, err := json.Marshal(struct {
		ID         uuid.UUID `json:"id"`
		PracticeID uuid.UUID `json:"practice_id"`
	}{ID: appointmentID, PracticeID: practiceID})
	if err != nil {
		return err
	}
	topic := "practice:" + practiceID.String()
	endpoint := baseURL + "/realtime/v1/api/broadcast/" + url.PathEscape(topic) + "/events/" + url.PathEscape(event)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("apikey", apiKey)
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Supabase Realtime returned %s", response.Status)
	}
	return nil
}

func (r *realtimeAppointmentService) SyncAppointmentCreated(context.Context, AppointmentEvent) error {
	return nil
}

func (r *realtimeAppointmentService) SyncAppointmentUpdated(context.Context, AppointmentEvent) error {
	return nil
}

func (r *realtimeAppointmentService) SyncAppointmentCancelled(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

type appointmentEventFanout struct {
	calendar *googleCalendarService
	realtime *realtimeAppointmentService
}

func (f *appointmentEventFanout) BroadcastAppointmentChange(ctx context.Context, practiceID uuid.UUID, event string, appointmentID uuid.UUID) error {
	return f.realtime.BroadcastAppointmentChange(ctx, practiceID, event, appointmentID)
}

func (f *appointmentEventFanout) SyncAppointmentCreated(ctx context.Context, event AppointmentEvent) error {
	return f.calendar.SyncAppointmentCreated(ctx, event)
}

func (f *appointmentEventFanout) SyncAppointmentUpdated(ctx context.Context, event AppointmentEvent) error {
	return f.calendar.SyncAppointmentUpdated(ctx, event)
}

func (f *appointmentEventFanout) SyncAppointmentCancelled(ctx context.Context, practiceID, appointmentID uuid.UUID) error {
	return f.calendar.SyncAppointmentCancelled(ctx, practiceID, appointmentID)
}

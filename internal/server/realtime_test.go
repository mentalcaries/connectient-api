package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRealtimeAppointmentBroadcastWireContract(t *testing.T) {
	practiceID := uuid.New()
	appointmentID := uuid.New()
	calls := 0
	service := &realtimeAppointmentService{
		env: func(key string) string {
			return map[string]string{
				"SUPABASE_URL":        "https://project.supabase.co/",
				"SUPABASE_SECRET_KEY": "secret-key",
			}[key]
		},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			wantPath := "/realtime/v1/api/broadcast/practice:" + practiceID.String() + "/events/UPDATE"
			if request.Method != http.MethodPost || request.URL.Path != wantPath || request.URL.RawQuery != "" {
				t.Errorf("request=%s %s?%s, want POST %s", request.Method, request.URL.Path, request.URL.RawQuery, wantPath)
			}
			if request.Header.Get("apikey") != "secret-key" || request.Header.Get("Authorization") != "Bearer secret-key" || request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("unexpected headers: %v", request.Header)
			}
			body, _ := io.ReadAll(request.Body)
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 2 || payload["id"] != appointmentID.String() || payload["practice_id"] != practiceID.String() {
				t.Errorf("non-minimal payload: %s", body)
			}
			return testHTTPResponse(http.StatusAccepted), nil
		})},
	}
	if err := service.BroadcastAppointmentChange(context.Background(), practiceID, "UPDATE", appointmentID); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRealtimeAppointmentBroadcastSafety(t *testing.T) {
	for _, flag := range []string{"E2E_TEST_MODE", "SKIP_REALTIME"} {
		t.Run(flag, func(t *testing.T) {
			calls := 0
			service := &realtimeAppointmentService{
				env: func(key string) string {
					if key == flag {
						return "true"
					}
					return ""
				},
				client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return testHTTPResponse(http.StatusOK), nil
				})},
			}
			if err := service.BroadcastAppointmentChange(context.Background(), uuid.New(), "INSERT", uuid.New()); err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatalf("safety flag made %d network calls", calls)
			}
		})
	}
}

func TestRealtimeAppointmentBroadcastRejectsInvalidConfigurationAndEvents(t *testing.T) {
	service := &realtimeAppointmentService{
		env: func(string) string { return "" },
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("network must not be called")
			return nil, nil
		})},
	}
	if err := service.BroadcastAppointmentChange(context.Background(), uuid.New(), "UPSERT", uuid.New()); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalid event error=%v", err)
	}
	if err := service.BroadcastAppointmentChange(context.Background(), uuid.New(), "INSERT", uuid.New()); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing config error=%v", err)
	}
}

func TestRealtimeAppointmentBroadcastProviderFailure(t *testing.T) {
	service := &realtimeAppointmentService{
		env: func(key string) string {
			if key == "SUPABASE_URL" {
				return "https://project.supabase.co"
			}
			return "configured"
		},
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return testHTTPResponse(http.StatusUnauthorized), nil
		})},
	}
	if err := service.BroadcastAppointmentChange(context.Background(), uuid.New(), "DELETE", uuid.New()); err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Fatalf("provider failure error=%v", err)
	}
}

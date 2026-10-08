package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestOutboundNotificationSafetyPreventsNetwork(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want RegistrationDeliveryResult
	}{
		{"test mode", map[string]string{"E2E_TEST_MODE": "true"}, RegistrationDeliverySimulated},
		{"all disabled", map[string]string{"OUTBOUND_MESSAGES_DISABLED": "true"}, RegistrationDeliveryUnavailable},
		{"email skipped", map[string]string{"SKIP_EMAIL": "true"}, RegistrationDeliveryUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			provider := testOutboundProvider(tc.env, func(*http.Request) (*http.Response, error) {
				calls++
				return testHTTPResponse(http.StatusOK), nil
			})
			result, err := provider.SendTeamInvite(context.Background(), TeamInviteNotification{
				Email: "invite@example.test", FirstName: "Jane", PracticeName: "Practice",
				InviterName: "Owner", Role: "staff", Link: "https://example.test/invite",
			})
			if err != nil || result != tc.want || calls != 0 {
				t.Errorf("result=%q err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestOutboundTestModeCoversEveryInterface(t *testing.T) {
	calls := 0
	provider := testOutboundProvider(map[string]string{"E2E_TEST_MODE": "true"}, func(*http.Request) (*http.Response, error) {
		calls++
		return testHTTPResponse(http.StatusOK), nil
	})
	practiceID := uuid.New()
	email, phone := "patient@example.test", "+15551111111"
	if result, err := provider.SendRegistrationEmail(context.Background(), RegistrationNotification{PracticeID: practiceID, PatientEmail: &email}); err != nil || result != RegistrationDeliverySimulated {
		t.Errorf("registration email=%q err=%v", result, err)
	}
	if result, err := provider.SendRegistrationWhatsApp(context.Background(), RegistrationNotification{PracticeID: practiceID, PatientPhone: &phone}); err != nil || result != RegistrationDeliverySimulated {
		t.Errorf("registration WhatsApp=%q err=%v", result, err)
	}
	if result, err := provider.SendAppointmentEmail(context.Background(), AppointmentNotification{PracticeID: practiceID, Email: email}); err != nil || result != RegistrationDeliverySimulated {
		t.Errorf("appointment email=%q err=%v", result, err)
	}
	if result, err := provider.SendAppointmentWhatsApp(context.Background(), AppointmentNotification{PracticeID: practiceID, MobilePhone: phone}); err != nil || result != RegistrationDeliverySimulated {
		t.Errorf("appointment WhatsApp=%q err=%v", result, err)
	}
	if err := provider.NotifyStaffAppointmentRequest(context.Background(), PublicAppointmentRequestNotification{PracticeID: practiceID}); err != nil {
		t.Errorf("public notification err=%v", err)
	}
	if calls != 0 {
		t.Errorf("test mode made %d network calls", calls)
	}
}

func TestOutboundProviderHTTPRequests(t *testing.T) {
	var mu sync.Mutex
	requests := make([]*http.Request, 0, 2)
	bodies := make([]string, 0, 2)
	provider := testOutboundProvider(map[string]string{
		"RESEND_API_KEY": "resend-secret", "FROM_EMAIL": "sender@example.test",
		"TWILIO_ACCOUNT_SID": "AC123", "TWILIO_AUTH_TOKEN": "twilio-secret",
		"TWILIO_WHATSAPP_NUMBER": "+15550000000",
	}, func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		requests, bodies = append(requests, request), append(bodies, string(body))
		mu.Unlock()
		return testHTTPResponse(http.StatusCreated), nil
	})

	result, err := provider.sendResend(context.Background(), resendMessage{
		From: "Practice <sender@example.test>", To: "patient@example.test",
		Subject: "Subject", HTML: "<p>Body</p>", Text: "Body",
	})
	if err != nil || result != RegistrationDeliverySent {
		t.Fatalf("resend result=%q err=%v", result, err)
	}
	result, err = provider.sendTwilio(context.Background(), "+15551111111", "HX123", map[string]string{"1": "Jane"})
	if err != nil || result != RegistrationDeliverySent {
		t.Fatalf("twilio result=%q err=%v", result, err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count=%d", len(requests))
	}
	if requests[0].URL.String() != resendEndpoint || requests[0].Header.Get("Authorization") != "Bearer resend-secret" ||
		!strings.Contains(bodies[0], `"to":"patient@example.test"`) {
		t.Errorf("unexpected Resend request: %s headers=%v body=%s", requests[0].URL, requests[0].Header, bodies[0])
	}
	username, password, ok := requests[1].BasicAuth()
	if !ok || username != "AC123" || password != "twilio-secret" ||
		!strings.Contains(requests[1].URL.Path, "/Accounts/AC123/Messages.json") ||
		!strings.Contains(bodies[1], "ContentSid=HX123") || !strings.Contains(bodies[1], "whatsapp%3A%2B15551111111") {
		t.Errorf("unexpected Twilio request: %s auth=%q/%q body=%s", requests[1].URL, username, password, bodies[1])
	}
}

func TestOutboundProviderMissingConfiguration(t *testing.T) {
	provider := testOutboundProvider(nil, func(*http.Request) (*http.Response, error) {
		t.Fatal("network must not be called")
		return nil, nil
	})
	if result, err := provider.sendResend(context.Background(), resendMessage{To: "patient@example.test"}); err != nil || result != RegistrationDeliveryUnavailable {
		t.Errorf("Resend result=%q err=%v", result, err)
	}
	if result, err := provider.sendTwilio(context.Background(), "+15551111111", "", map[string]string{"1": "Jane"}); err != nil || result != RegistrationDeliveryUnavailable {
		t.Errorf("Twilio result=%q err=%v", result, err)
	}
}

func TestRegistrationTokenFromLink(t *testing.T) {
	if got := registrationToken("https://patient.example.test/register/token-123"); got != "token-123" {
		t.Errorf("token=%q", got)
	}
}

func testOutboundProvider(values map[string]string, transport roundTripFunc) *outboundNotificationProvider {
	return &outboundNotificationProvider{
		client: &http.Client{Transport: transport},
		env: func(key string) string {
			return values[key]
		},
	}
}

func testHTTPResponse(status int) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}
}

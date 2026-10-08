package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDecodeAcceptInvite(t *testing.T) {
	token := uuid.NewString()
	input, err := decodeAcceptInvite(strings.NewReader(`{
		"token":"` + token + `","first_name":" Jane ","last_name":" Doe ",
		"mobile_phone":" +15551234567 ","termsAgreed":true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.Token != token || input.FirstName != "Jane" || input.LastName != "Doe" ||
		input.MobilePhone == nil || *input.MobilePhone != "+15551234567" || !input.TermsAgreed {
		t.Errorf("unexpected input: %+v", input)
	}

	for _, body := range []string{
		`{}`,
		`{"token":"bad","first_name":"A","last_name":"B","termsAgreed":true}`,
		`{"token":"` + token + `","first_name":"A","last_name":"B","termsAgreed":false}`,
		`{"token":"` + token + `","first_name":"A","last_name":"B","mobile_phone":"123456789012345678901","termsAgreed":true}`,
	} {
		if _, err := decodeAcceptInvite(strings.NewReader(body)); err == nil {
			t.Errorf("expected validation error for %s", body)
		}
	}
}

func TestHTTPIdentityProfileService(t *testing.T) {
	userID := uuid.New()
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer service-secret" ||
			r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: method=%s auth=%q content-type=%q", r.Method, r.Header.Get("Authorization"), r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"user_id":"`+userID.String()+`"`) || !strings.Contains(string(body), `"name":"Jane Doe"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	service := &httpIdentityProfileService{url: server.URL, token: "service-secret", client: server.Client()}
	if err := service.UpdateDisplayName(context.Background(), userID, "Jane Doe"); err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("identity service was not called")
	}

	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer failingServer.Close()
	service.url = failingServer.URL
	service.client = failingServer.Client()
	if err := service.UpdateDisplayName(context.Background(), userID, "Jane Doe"); err == nil {
		t.Fatal("expected non-2xx identity service response to fail")
	}
}

func TestIdentityProfileServiceFromEnvRequiresURLAndToken(t *testing.T) {
	t.Setenv("AUTH_PROFILE_SERVICE_URL", "")
	t.Setenv("AUTH_PROFILE_SERVICE_TOKEN", "")
	if service := newHTTPIdentityProfileServiceFromEnv(); service != nil {
		t.Fatal("expected unconfigured service to be nil")
	}
	t.Setenv("AUTH_PROFILE_SERVICE_URL", "https://auth.example.test/internal/profile")
	if service := newHTTPIdentityProfileServiceFromEnv(); service != nil {
		t.Fatal("expected missing token to disable service")
	}
}

var errFakeIdentitySync = fmt.Errorf("fake identity sync failed")

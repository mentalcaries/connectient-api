package server

import (
	"strings"
	"testing"
)

func TestDecodePatchPatient(t *testing.T) {
	input, err := decodePatchPatient(strings.NewReader(`{
		"first_name":" Jane ","last_name":" Doe ","email":" jane@example.test ",
		"mobile_phone":"+15555550100","date_of_birth":"1990-04-05",
		"email_consent":false,"whatsapp_consent":true,
		"id":"ignored","practice_id":"ignored","updated_at":"ignored","force":true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	p := input.params
	if !p.SetFirstName || p.FirstName != "Jane" || !p.SetLastName || p.LastName != "Doe" ||
		p.Email == nil || *p.Email != "jane@example.test" || p.MobilePhone == nil ||
		p.DateOfBirth == nil || p.DateOfBirth.Format("2006-01-02") != "1990-04-05" ||
		!p.SetEmailConsent || p.EmailConsent || !p.SetWhatsappConsent || !p.WhatsappConsent {
		t.Errorf("unexpected input: %+v", p)
	}

	cleared, err := decodePatchPatient(strings.NewReader(`{"email":" ","mobile_phone":null,"date_of_birth":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !cleared.params.SetEmail || cleared.params.Email != nil || !cleared.params.SetMobilePhone ||
		cleared.params.MobilePhone != nil || !cleared.params.SetDateOfBirth || cleared.params.DateOfBirth != nil {
		t.Errorf("unexpected clear input: %+v", cleared.params)
	}

	for _, tc := range []struct {
		body, want string
	}{
		{`{"first_name":" "}`, "Invalid first_name"},
		{`{"email":"not-email"}`, "Invalid email"},
		{`{"mobile_phone":"123"}`, "Invalid mobile_phone"},
		{`{"date_of_birth":"04/05/1990"}`, "Invalid date_of_birth"},
		{`{"email_consent":null}`, "Invalid email_consent"},
	} {
		if _, err := decodePatchPatient(strings.NewReader(tc.body)); err == nil || err.Error() != tc.want {
			t.Errorf("body %s error = %v, want %q", tc.body, err, tc.want)
		}
	}
}

func TestSanitizePatientSearch(t *testing.T) {
	if got := sanitizePatientSearch("  Jane%_,()  Doe "); got != "Jane Doe" {
		t.Errorf("sanitizePatientSearch = %q", got)
	}
}

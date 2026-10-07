package server

import (
	"strings"
	"testing"
)

func TestDecodeCreateRegistration(t *testing.T) {
	input, err := decodeCreateRegistration(strings.NewReader(`{
		"patient_name":" Jane Doe ","patient_email":" jane@example.test ",
		"patient_phone":"+15555550100","send_email":true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.PatientName != "Jane Doe" || input.PatientEmail == nil || *input.PatientEmail != "jane@example.test" ||
		input.PatientPhone == nil || *input.PatientPhone != "+15555550100" || !input.SendEmail || input.SendWhatsApp {
		t.Errorf("unexpected input: %+v", input)
	}

	for _, tc := range []struct {
		body, want string
	}{
		{`{}`, "patient_name is required"},
		{`{"patient_name":"Jane","send_email":true,"send_whatsapp":true}`, "Choose either email or WhatsApp, not both"},
		{`{"patient_name":"Jane","appointment_id":"bad"}`, "Invalid appointment ID"},
		{`{"patient_name":"Jane","send_email":null}`, "Invalid send_email"},
	} {
		if _, err := decodeCreateRegistration(strings.NewReader(tc.body)); err == nil || err.Error() != tc.want {
			t.Errorf("body %s error = %v, want %q", tc.body, err, tc.want)
		}
	}
}

func TestRegistrationToken(t *testing.T) {
	first, err := newRegistrationToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newRegistrationToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || first == second {
		t.Errorf("unexpected tokens: %q %q", first, second)
	}
}

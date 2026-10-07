package server

import (
	"strings"
	"testing"
)

func TestDecodePublicAppointmentRequest(t *testing.T) {
	input, err := decodePublicAppointmentRequest(strings.NewReader(`{
		"first_name":" Jane ","last_name":" Doe ","email":" JANE@EXAMPLE.TEST ",
		"mobile_phone":" +1 (555) 123-4567 ","requested_date":"2099-10-08",
		"requested_time":"flexible","appointment_type":" consultation ",
		"description":" Checkup ","is_emergency":false
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.FirstName != "Jane" || input.LastName != "Doe" || input.Email != "jane@example.test" ||
		input.AppointmentType != "consultation" || input.Description == nil || *input.Description != "Checkup" {
		t.Errorf("unexpected normalized input: %+v", input)
	}

	for _, body := range []string{
		`{}`,
		`{"first_name":"Jane","last_name":"Doe","email":"bad","mobile_phone":"15551234567","requested_date":"2099-10-08","requested_time":"flexible","appointment_type":"consultation"}`,
		`{"first_name":"Jane","last_name":"Doe","email":"jane@example.test","mobile_phone":"123","requested_date":"2099-10-08","requested_time":"flexible","appointment_type":"consultation"}`,
		`{"first_name":"Jane","last_name":"Doe","email":"jane@example.test","mobile_phone":"15551234567","requested_date":"2099-10-08","requested_time":"night","appointment_type":"consultation"}`,
		`{"first_name":"Jane","last_name":"Doe","email":"jane@example.test","mobile_phone":"15551234567","requested_date":"2099-10-08","requested_time":"flexible","appointment_type":"consultation","unknown":true}`,
	} {
		if _, err := decodePublicAppointmentRequest(strings.NewReader(body)); err == nil {
			t.Errorf("expected invalid request: %s", body)
		}
	}
}

func TestPublicIdempotencyKeyPattern(t *testing.T) {
	for _, value := range []string{"request-123", "550e8400-e29b-41d4-a716-446655440000"} {
		if !publicIdempotencyKeyPattern.MatchString(value) {
			t.Errorf("valid key rejected: %q", value)
		}
	}
	for _, value := range []string{"short", "spaces are invalid", strings.Repeat("x", 129)} {
		if publicIdempotencyKeyPattern.MatchString(value) {
			t.Errorf("invalid key accepted: %q", value)
		}
	}
}

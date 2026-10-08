package server

import (
	"strings"
	"testing"
)

const validRegistrationSubmission = `{
  "form_version":"medical-v1",
  "form_data":{
    "personal":{
      "surname":" Doe ","first_name":" Jane ","date_of_birth":"1990-04-05",
      "sex":"female","address_line_1":"1 Main St","address_line_2":"",
      "city":"Test City","mobile_phone":"+1 (555) 555-0100","home_phone":"",
      "email":" JANE@EXAMPLE.TEST ","email_contact_ok":false
    },
    "additional":{
      "marital_status":"single","emergency_contact_name":"John Doe",
      "emergency_contact_phone":"+15555550101"
    },
    "medical_history":{"has_conditions":false},
    "signature":{"signed_by":"Jane Doe"}
  }
}`

func TestDecodePublicRegistrationSubmission(t *testing.T) {
	input, err := decodePublicRegistrationSubmission(strings.NewReader(validRegistrationSubmission))
	if err != nil {
		t.Fatal(err)
	}
	if input.FirstName != "Jane" || input.LastName != "Doe" || input.Email != "jane@example.test" ||
		input.MobilePhone != "15555550100" || input.DateOfBirth.Format("2006-01-02") != "1990-04-05" ||
		input.EmailConsent || input.AddressLine1 == nil || *input.AddressLine1 != "1 Main St" {
		t.Errorf("unexpected submission: %+v", input)
	}

	for _, body := range []string{
		`{}`,
		`{"form_version":"v1","form_data":{}}`,
		strings.Replace(validRegistrationSubmission, `"surname":" Doe "`, `"surname":" "`, 1),
		strings.Replace(validRegistrationSubmission, `"date_of_birth":"1990-04-05"`, `"date_of_birth":"bad"`, 1),
		strings.Replace(validRegistrationSubmission, `"email":" JANE@EXAMPLE.TEST "`, `"email":"bad"`, 1),
		strings.Replace(validRegistrationSubmission, `"signed_by":"Jane Doe"`, `"signed_by":""`, 1),
	} {
		if _, err := decodePublicRegistrationSubmission(strings.NewReader(body)); err == nil {
			t.Errorf("expected validation error for %s", body)
		}
	}
}

func TestValidRegistrationToken(t *testing.T) {
	if !validRegistrationToken(strings.Repeat("a", 64)) || validRegistrationToken(strings.Repeat("g", 64)) || validRegistrationToken("short") {
		t.Error("unexpected token validation")
	}
}

package server

import (
	"strings"
	"testing"
)

func TestDecodeCreatePracticeInvite(t *testing.T) {
	input, err := decodeCreatePracticeInvite(strings.NewReader(`{
		"first_name":" Jane ","last_name":" Doe ","email":" JANE@EXAMPLE.TEST ",
		"role":"admin","org_role":" Manager "
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.FirstName != "Jane" || input.LastName != "Doe" || input.Email != "jane@example.test" ||
		input.Role != "admin" || input.OrgRole == nil || *input.OrgRole != "Manager" {
		t.Errorf("unexpected input: %+v", input)
	}
	for _, body := range []string{`{}`, `{"first_name":"A","last_name":"B","email":"bad","role":"staff"}`, `{"first_name":"A","last_name":"B","email":"a@example.test","role":"owner"}`} {
		if _, err := decodeCreatePracticeInvite(strings.NewReader(body)); err == nil {
			t.Errorf("expected error for %s", body)
		}
	}
}

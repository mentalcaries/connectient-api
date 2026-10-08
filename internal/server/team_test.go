package server

import (
	"strings"
	"testing"
)

func TestDecodePatchPracticeUser(t *testing.T) {
	input, err := decodePatchPracticeUser(strings.NewReader(`{"role":"admin","is_active":false,"practice_id":"ignored"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !input.SetRole || input.Role != "admin" || !input.SetIsActive || input.IsActive {
		t.Errorf("unexpected input: %+v", input)
	}
	for _, body := range []string{`{}`, `{"role":"owner"}`, `{"is_active":null}`, `null`} {
		if _, err := decodePatchPracticeUser(strings.NewReader(body)); err == nil {
			t.Errorf("expected validation error for %s", body)
		}
	}
}

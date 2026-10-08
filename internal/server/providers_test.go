package server

import (
	"strings"
	"testing"
)

func TestDecodeCreateProvider(t *testing.T) {
	input, err := decodeCreateProvider(strings.NewReader(`{
		"first_name":"  Jane ","last_name":" Doe  ","title":" Dr ",
		"specialty":" Orthodontics ","is_main":true
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.FirstName != "Jane" || input.LastName != "Doe" || input.Title == nil || *input.Title != "Dr" ||
		input.Specialty != "Orthodontics" || !input.IsMain {
		t.Errorf("unexpected input: %+v", input)
	}

	defaults, err := decodeCreateProvider(strings.NewReader(`{"first_name":"Jane","last_name":"Doe","title":"","specialty":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Title != nil || defaults.Specialty != "General" {
		t.Errorf("unexpected defaults: %+v", defaults)
	}

	for _, tc := range []struct {
		body, want string
	}{
		{`{"last_name":"Doe"}`, "First name and last name are required"},
		{`{"first_name":" ","last_name":"Doe"}`, "First name and last name are required"},
		{`{"first_name":"Jane","last_name":"Doe","is_main":null}`, "Invalid is_main"},
	} {
		if _, err := decodeCreateProvider(strings.NewReader(tc.body)); err == nil || err.Error() != tc.want {
			t.Errorf("body %s error = %v, want %q", tc.body, err, tc.want)
		}
	}
}

func TestDecodePatchProvider(t *testing.T) {
	input, err := decodePatchProvider(strings.NewReader(`{
		"first_name":" Jane ","last_name":" Doe ","title":" ","specialty":null
	}`))
	if err != nil {
		t.Fatal(err)
	}
	params := input.params
	if !input.hasUpdates() || params.FirstName != "Jane" || params.LastName != "Doe" ||
		params.Title != nil || params.Specialty != "General" {
		t.Errorf("unexpected patch: %+v", params)
	}
	if _, err := decodePatchProvider(strings.NewReader(`{"first_name":" "}`)); err == nil || err.Error() != "Invalid first_name" {
		t.Errorf("error = %v, want invalid first_name", err)
	}
}

package server

import (
	"strings"
	"testing"
)

func TestDecodeAccountPatch(t *testing.T) {
	input, err := decodeAccountPatch(strings.NewReader(`{
		"first_name":" Jane ","last_name":" Doe ","avatar_url":null,
		"mobile_phone":null,"whatsapp_notifications_enabled":false
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.FirstName != "Jane" || input.LastName != "Doe" || input.AvatarURL != nil ||
		!input.SetMobilePhone || input.MobilePhone != nil || !input.SetWhatsappNotificationsEnabled || input.WhatsappNotificationsEnabled {
		t.Errorf("unexpected input: %+v", input)
	}

	for _, tc := range []struct {
		body, want string
	}{
		{`{"last_name":"Doe"}`, "First name is required"},
		{`{"first_name":"Jane","last_name":" "}`, "Last name is required"},
		{`{"first_name":"Jane","last_name":"Doe","avatar_url":12}`, "Invalid avatar_url"},
		{`{"first_name":"Jane","last_name":"Doe","mobile_phone":12}`, "Invalid mobile_phone"},
		{`{"first_name":"Jane","last_name":"Doe","whatsapp_notifications_enabled":null}`, "Invalid whatsapp_notifications_enabled"},
	} {
		if _, err := decodeAccountPatch(strings.NewReader(tc.body)); err == nil || err.Error() != tc.want {
			t.Errorf("body %s error = %v, want %q", tc.body, err, tc.want)
		}
	}
}

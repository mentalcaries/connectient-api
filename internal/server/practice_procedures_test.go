package server

import (
	"strings"
	"testing"
)

func TestDecodeCreateProcedureType(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        string
		wantPrimary bool
		wantErr     string
	}{
		{name: "required fields", body: `{"name":"Consultation","value":"consultation"}`},
		{name: "primary", body: `{"name":"Consultation","value":"consultation","is_primary":true}`, wantPrimary: true},
		{name: "missing value", body: `{"name":"Consultation"}`, wantErr: "Name and value are required"},
		{name: "empty name", body: `{"name":"","value":"consultation"}`, wantErr: "Name and value are required"},
		{name: "invalid primary", body: `{"name":"Consultation","value":"consultation","is_primary":null}`, wantErr: "Invalid is_primary"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeCreateProcedureType(strings.NewReader(tc.body))
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "Consultation" || got.Value != "consultation" || got.IsPrimary != tc.wantPrimary {
				t.Errorf("input = %+v", got)
			}
		})
	}
}

func TestDecodePatchProcedureType(t *testing.T) {
	input, err := decodePatchProcedureType(strings.NewReader(`{"name":"Review","is_active":false,"sort_order":4}`))
	if err != nil {
		t.Fatal(err)
	}
	params := input.params
	if !input.hasUpdates() || !params.SetName || params.Name != "Review" ||
		!params.SetIsActive || params.IsActive || !params.SetSortOrder || params.SortOrder != 4 {
		t.Errorf("unexpected patch: %+v", params)
	}
	for _, tc := range []struct {
		body, want string
	}{
		{`{"name":null}`, "Invalid name"},
		{`{"is_active":"false"}`, "Invalid is_active"},
		{`{"sort_order":1.5}`, "Invalid sort_order"},
	} {
		if _, err := decodePatchProcedureType(strings.NewReader(tc.body)); err == nil || err.Error() != tc.want {
			t.Errorf("body %s error = %v, want %q", tc.body, err, tc.want)
		}
	}
}

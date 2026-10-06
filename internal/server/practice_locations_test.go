package server

import (
	"slices"
	"strings"
	"testing"
)

func TestDecodeCreatePracticeLocation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        string
		wantName    string
		wantAddress *string
		wantErr     string
	}{
		{name: "name only", body: `{"name":"Main"}`, wantName: "Main"},
		{name: "with address", body: `{"name":"Main","address":"1 Test Road"}`, wantName: "Main", wantAddress: stringPointer("1 Test Road")},
		{name: "empty address becomes null", body: `{"name":"Main","address":""}`, wantName: "Main"},
		{name: "missing name", body: `{"address":"Test"}`, wantErr: "Name is required"},
		{name: "empty name", body: `{"name":""}`, wantErr: "Name is required"},
		{name: "invalid address", body: `{"name":"Main","address":12}`, wantErr: "Invalid address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeCreatePracticeLocation(strings.NewReader(tc.body))
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != tc.wantName || !equalOptionalString(got.Address, tc.wantAddress) {
				t.Errorf("input = %+v", got)
			}
		})
	}
}

func TestDecodePatchPracticeLocation(t *testing.T) {
	validTime := "2026-10-06T12:00:00Z"
	input, err := decodePatchPracticeLocation(strings.NewReader(`{
		"name":"Branch","address":null,"is_active":false,"sort_order":3,
		"deleted_at":"` + validTime + `","available_weekdays":[6,1,1]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	params := input.params
	if !input.hasUpdates() || !params.SetName || params.Name != "Branch" || !params.SetAddress || params.Address != nil ||
		!params.SetIsActive || params.IsActive || !params.SetSortOrder || params.SortOrder != 3 ||
		!params.SetDeletedAt || !params.DeletedAt.Valid ||
		!params.SetAvailableWeekdays || !slices.Equal(params.AvailableWeekdays, []int16{1, 6}) {
		t.Errorf("unexpected patch: %+v", params)
	}

	for _, tc := range []struct {
		body, want string
	}{
		{`{"name":null}`, "Invalid name"},
		{`{"is_active":null}`, "Invalid is_active"},
		{`{"sort_order":1.5}`, "Invalid sort_order"},
		{`{"deleted_at":"yesterday"}`, "Invalid deleted_at"},
		{`{"available_weekdays":[7]}`, "Invalid available weekdays"},
	} {
		if _, err := decodePatchPracticeLocation(strings.NewReader(tc.body)); err == nil || err.Error() != tc.want {
			t.Errorf("body %s error = %v, want %q", tc.body, err, tc.want)
		}
	}
}

func stringPointer(value string) *string { return &value }

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

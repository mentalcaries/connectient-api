package server

import "encoding/json"

type DefaultProcedureType struct {
	Name      string
	Value     string
	SortOrder int
	IsPrimary bool
}

type DefaultPracticeSettings struct {
	DentalHistoryEnabled        bool
	TMJHistoryEnabled           bool
	MultipleLocationsEnabled    bool
	CustomFormSections          json.RawMessage
	PhysiotherapyHistoryEnabled bool
	OptometryHistoryEnabled     bool
}

var DefaultProcedureTypes = map[string][]DefaultProcedureType{
	"dental": {
		{Name: "Consultation", Value: "consultation", SortOrder: 1, IsPrimary: true},
		{Name: "Cleaning/Polishing", Value: "cleaning", SortOrder: 2, IsPrimary: false},
		{Name: "Extraction", Value: "extraction", SortOrder: 3, IsPrimary: false},
		{Name: "Filling", Value: "filling", SortOrder: 4, IsPrimary: false},
		{Name: "Something Else", Value: "other", SortOrder: 5, IsPrimary: false},
	},
	"medical": {
		{Name: "Consultation", Value: "consultation", SortOrder: 1, IsPrimary: true},
		{Name: "Review", Value: "review", SortOrder: 2, IsPrimary: false},
	},
	"optometry": {
		{Name: "Routine Eye Examination", Value: "routine_eye_exam", SortOrder: 1, IsPrimary: true},
		{Name: "Contact Lens Fitting", Value: "contact_lens_fitting", SortOrder: 2, IsPrimary: false},
		{Name: "Glasses Prescription", Value: "glasses_prescription", SortOrder: 3, IsPrimary: false},
		{Name: "Emergency Eye Consultation", Value: "emergency_eye_consult", SortOrder: 4, IsPrimary: false},
	},
	"physiotherapy": {
		{Name: "Initial Assessment / Consultation", Value: "initial_assessment", SortOrder: 1, IsPrimary: true},
		{Name: "Follow-up Session", Value: "follow_up", SortOrder: 2, IsPrimary: false},
		{Name: "Manual Therapy", Value: "manual_therapy", SortOrder: 3, IsPrimary: false},
		{Name: "Exercise Therapy", Value: "exercise_therapy", SortOrder: 4, IsPrimary: false},
	},
}

var DefaultSettingsByCategory = map[string]DefaultPracticeSettings{
	"dental":        {DentalHistoryEnabled: true},
	"medical":       {},
	"optometry":     {OptometryHistoryEnabled: true},
	"physiotherapy": {PhysiotherapyHistoryEnabled: true},
}

# Practice Domain — Go Migration Plan

## 0. Registration (Onboarding)

**Handler:** `handlerNewRegistration`

| Handler | Route | Notes |
|---|---|---|
| `handlerNewRegistration` | `POST /register` | Single transaction: creates User, Practice, practice_settings, procedure_types, subscription, in that order. Compensating rollback on failure. |

**Structs (consumed by the service function called from this handler):**
```go
type CreateUserParams struct {
	ID            uuid.UUID
	Email         string
	FirstName     string
	LastName      string
	MobilePhone   string
	PracticeID    *uuid.UUID
	Role          string
	TermsAgreedAt time.Time
}

type CreatePracticeParams struct {
	Name                 string  `json:"name"`
	Email                string  `json:"email"`
	PracticeCategory     string  `json:"practice_category"`
	Specialty            *string `json:"specialty"`
	PracticeCode         string  `json:"practice_code"`
	City                 string  `json:"city"`
	HasMultipleProviders bool    `json:"has_multiple_providers"`
}

type CreatePracticeSettingsParams struct {
	PracticeID                  uuid.UUID        `json:"practice_id"`
	DentalHistoryEnabled        bool             `json:"dental_history_enabled"`
	TMJHistoryEnabled           bool             `json:"tmj_history_enabled"`
	MultipleLocationsEnabled    bool             `json:"multiple_locations_enabled"`
	OptometryHistoryEnabled     bool             `json:"optometry_history_enabled"`
	PhysiotherapyHistoryEnabled bool             `json:"physiotherapy_history_enabled"`
	CustomFormSections          *json.RawMessage `json:"custom_form_sections"`
	Theme                       string           `json:"theme"`
	ThemeColors                 *json.RawMessage `json:"theme_colors"`
}

type CreateProcedureTypeParams struct {
	PracticeID uuid.UUID `json:"practice_id"`
	Name       string    `json:"name"`
	Value      string    `json:"value"`
	IsPrimary  bool      `json:"is_primary"`
}

type CreateSubscriptionParams struct {
	ReferenceID uuid.UUID `json:"reference_id"`
	Plan        string    `json:"plan"`
	Status      string    `json:"status"`
	TrialStart  time.Time `json:"trial_start"`
	TrialEnd    time.Time `json:"trial_end"`
}
```

Note: `UpdateUserPracticeParams` (sets `practice_id` on User after Practice insert) already established separately, used mid-transaction here.

---

## 1. Practices

**Controller:** `PracticeController`

| Handler | Route | Notes |
|---|---|---|
| `GetPracticeProfile` | `GET /api/practice` | Combined read: practice + settings + procedure_types + locations + `is_admin` |
| `UpdatePractice` | `PATCH /api/practice` | Single handler/struct covers both existing Next.js update paths |

```go
type UpdatePracticeParams struct {
	Name                 *string `json:"name,omitempty"`
	StreetAddress        *string `json:"street_address,omitempty"`
	City                 *string `json:"city,omitempty"`
	Phone                *string `json:"phone,omitempty"`
	Email                *string `json:"email,omitempty"`
	Website              *string `json:"website,omitempty"`
	Facebook             *string `json:"facebook,omitempty"`
	Instagram            *string `json:"instagram,omitempty"`
	Specialty            *string `json:"specialty,omitempty"`
	PracticeCode         *string `json:"practice_code,omitempty"`
	HasMultipleProviders *bool   `json:"has_multiple_providers,omitempty"`
}
```

## 2. Practice Logo

**Controller:** `PracticeLogoController`

| Handler | Route | Notes |
|---|---|---|
| `UploadLogo` | `POST /api/practice/logo` | Multipart; R2 |
| `RemoveLogo` | `DELETE /api/practice/logo` | Clears `Practice.logo` |

No params struct — multipart fields read directly.

## 3. Practice Settings

**Controller:** `PracticeSettingsController`

| Handler | Route |
|---|---|
| `UpdatePracticeSettings` | `PATCH /api/practice/settings` |

```go
type UpdatePracticeSettingsParams struct {
	DentalHistoryEnabled        *bool            `json:"dental_history_enabled,omitempty"`
	TMJHistoryEnabled           *bool            `json:"tmj_history_enabled,omitempty"`
	MultipleLocationsEnabled    *bool            `json:"multiple_locations_enabled,omitempty"`
	OptometryHistoryEnabled     *bool            `json:"optometry_history_enabled,omitempty"`
	PhysiotherapyHistoryEnabled *bool            `json:"physiotherapy_history_enabled,omitempty"`
	CustomFormSections          *json.RawMessage `json:"custom_form_sections,omitempty"`
	Theme                       *string          `json:"theme,omitempty"`
	ThemeColors                 *json.RawMessage `json:"theme_colors,omitempty"`
}
```

## 4. Practice Locations

**Controller:** `PracticeLocationController`

| Handler | Route | Notes |
|---|---|---|
| `ListLocations` | `GET /api/practice/locations` | Excludes `deleted_at IS NOT NULL`, ordered by `sort_order` |
| `CreateLocation` | `POST /api/practice/locations` | `address` required; auto-computes next `sort_order` |
| `UpdateLocation` | `PATCH /api/practice/locations/:id` | Allowed fields: name, address, is_active, sort_order, deleted_at |

```go
type CreateLocationParams struct {
	PracticeID uuid.UUID `json:"practice_id"`
	Name       string    `json:"name"`
	Address    string    `json:"address"`
}

type UpdateLocationParams struct {
	Name      *string    `json:"name,omitempty"`
	Address   *string    `json:"address,omitempty"`
	IsActive  *bool      `json:"is_active,omitempty"`
	SortOrder *int       `json:"sort_order,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
```

## 5. Procedure Types

**Controller:** `ProcedureTypeController`

| Handler | Route | Notes |
|---|---|---|
| `ListProcedureTypes` | `GET /api/practice/procedure-types` | Excludes `deleted_at`, ordered by `sort_order` |
| `CreateProcedureType` | `POST /api/practice/procedure-types` | Checks `(practice_id, value)` uniqueness; unsets prior `is_primary` if new one is primary |
| `UpdateProcedureType` | `PATCH /api/practice/procedure-types/:id` | Blocks name edit if `is_default` |
| `SoftDeleteProcedureType` | `DELETE /api/practice/procedure-types/:id` | Blocks delete if `is_default` |

```go
type UpdateProcedureTypeParams struct {
	Name      *string `json:"name,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
}
```
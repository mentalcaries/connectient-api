package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/mentalcaries/connectient-api/internal/database"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

const googleCalendarScope = "https://www.googleapis.com/auth/calendar.app.created email profile"

type googleCalendarService struct {
	db      database.Service
	queries *db.Queries
	client  *http.Client
	env     func(string) string
	cipher  *tokenCipher
}

func newGoogleCalendarService(databaseService database.Service, queries *db.Queries) *googleCalendarService {
	cipher, _ := newTokenCipher(os.Getenv("ENCRYPTION_KEY"))
	return &googleCalendarService{
		db: databaseService, queries: queries, client: &http.Client{Timeout: 10 * time.Second},
		env: os.Getenv, cipher: cipher,
	}
}

func (g *googleCalendarService) Disabled() bool {
	return g.env("E2E_TEST_MODE") == "true" || g.env("SKIP_CALENDAR") == "true"
}

func (g *googleCalendarService) BroadcastAppointmentChange(context.Context, uuid.UUID, string, uuid.UUID) error {
	// Calendar-only test wiring has no invalidation transport. Production uses
	// appointmentEventFanout to publish the same mutation to the SSE hub.
	return nil
}

func (g *googleCalendarService) SyncAppointmentCreated(ctx context.Context, event AppointmentEvent) error {
	if g.Disabled() {
		return nil
	}
	return g.withPracticeLock(ctx, event.PracticeID, func() error {
		if _, err := g.queries.GetAppointmentCalendarMapping(ctx, event.AppointmentID); err == nil {
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return g.createEvent(ctx, event)
	})
}

func (g *googleCalendarService) SyncAppointmentUpdated(ctx context.Context, event AppointmentEvent) error {
	if g.Disabled() {
		return nil
	}
	return g.withPracticeLock(ctx, event.PracticeID, func() error {
		mapping, err := g.queries.GetAppointmentCalendarMapping(ctx, event.AppointmentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return g.createEvent(ctx, event)
		}
		if err != nil {
			return err
		}
		connection, token, calendarID, err := g.connectionTokenCalendar(ctx, event.PracticeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		status, err := g.sendEvent(ctx, http.MethodPatch, calendarID, mapping, token, event)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 {
			return nil
		}
		if status != http.StatusNotFound {
			return fmt.Errorf("Google event update returned %d", status)
		}
		calendarExists, err := g.googleCalendarExists(ctx, calendarID, token)
		if err != nil {
			return err
		}
		if !calendarExists {
			if err := g.queries.SetGoogleAppCalendar(ctx, db.SetGoogleAppCalendarParams{AppCalendarID: nil, ID: connection.ID}); err != nil {
				return err
			}
		}
		return g.createEvent(ctx, event)
	})
}

func (g *googleCalendarService) SyncAppointmentCancelled(ctx context.Context, practiceID, appointmentID uuid.UUID) error {
	if g.Disabled() {
		return nil
	}
	return g.withPracticeLock(ctx, practiceID, func() error {
		mapping, err := g.queries.GetAppointmentCalendarMapping(ctx, appointmentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		connection, token, calendarID, err := g.connectionTokenCalendar(ctx, practiceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return g.queries.DeleteAppointmentCalendarMapping(ctx, appointmentID)
		}
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodDelete, googleEventURL(calendarID, mapping), nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := g.client.Do(request)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusNotFound {
			return fmt.Errorf("Google event deletion returned %d", response.StatusCode)
		}
		if response.StatusCode == http.StatusNotFound {
			exists, checkErr := g.googleCalendarExists(ctx, calendarID, token)
			if checkErr == nil && !exists {
				_ = g.queries.SetGoogleAppCalendar(ctx, db.SetGoogleAppCalendarParams{AppCalendarID: nil, ID: connection.ID})
			}
		}
		return g.queries.DeleteAppointmentCalendarMapping(ctx, appointmentID)
	})
}

func (g *googleCalendarService) Backfill(ctx context.Context, practiceID uuid.UUID) error {
	if g.Disabled() {
		return nil
	}
	rows, err := g.queries.ListGoogleCalendarBackfillAppointments(ctx, practiceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ScheduledDate == nil || row.ScheduledTime == nil {
			continue
		}
		event := AppointmentEvent{
			AppointmentID: row.ID, PracticeID: practiceID,
			PatientName:   strings.TrimSpace(row.FirstName + " " + row.LastName),
			ProcedureType: pointerString(row.AppointmentType), Phone: row.MobilePhone,
			ScheduledDate: row.ScheduledDate.Format("2006-01-02"), ScheduledTime: trimCalendarTime(*row.ScheduledTime),
			Timezone: row.ScheduledTimezone, DurationMinutes: int(pointerInt32(row.DurationMinutes, 15)),
		}
		if err := g.SyncAppointmentCreated(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (g *googleCalendarService) createEvent(ctx context.Context, event AppointmentEvent) error {
	_, token, calendarID, err := g.connectionTokenCalendar(ctx, event.PracticeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	status, responseBody, err := g.sendEventForCreate(ctx, calendarID, token, event)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		connection, connectionErr := g.queries.GetGoogleConnection(ctx, event.PracticeID)
		if connectionErr != nil {
			return connectionErr
		}
		if err := g.queries.SetGoogleAppCalendar(ctx, db.SetGoogleAppCalendarParams{AppCalendarID: nil, ID: connection.ID}); err != nil {
			return err
		}
		_, token, calendarID, err = g.connectionTokenCalendar(ctx, event.PracticeID)
		if err != nil {
			return err
		}
		status, responseBody, err = g.sendEventForCreate(ctx, calendarID, token, event)
	}
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("Google event creation returned %d", status)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &created); err != nil || created.ID == "" {
		return errors.New("Google event creation returned no ID")
	}
	return g.queries.UpsertAppointmentCalendarMapping(ctx, db.UpsertAppointmentCalendarMappingParams{
		AppointmentID: event.AppointmentID, ExternalEventID: created.ID,
	})
}

func (g *googleCalendarService) connectionTokenCalendar(ctx context.Context, practiceID uuid.UUID) (db.ConnectedApp, string, string, error) {
	connection, err := g.queries.GetGoogleConnection(ctx, practiceID)
	if err != nil {
		return db.ConnectedApp{}, "", "", err
	}
	token, err := g.validAccessToken(ctx, connection)
	if err != nil {
		return db.ConnectedApp{}, "", "", err
	}
	calendarID, err := g.ensureAppCalendar(ctx, connection, token)
	return connection, token, calendarID, err
}

func (g *googleCalendarService) validAccessToken(ctx context.Context, connection db.ConnectedApp) (string, error) {
	if g.cipher == nil || connection.AccessToken == nil {
		return "", errors.New("Google token encryption is unavailable")
	}
	if connection.TokenExpiresAt != nil && connection.TokenExpiresAt.After(time.Now().Add(time.Minute)) {
		return g.cipher.Decrypt(*connection.AccessToken)
	}
	if connection.RefreshToken == nil {
		_ = g.queries.MarkGoogleConnectionFailed(ctx, db.MarkGoogleConnectionFailedParams{LastError: calendarStringPointer("No refresh token available"), ID: connection.ID})
		return "", errors.New("no Google refresh token available")
	}
	refreshToken, err := g.cipher.Decrypt(*connection.RefreshToken)
	if err != nil {
		return "", err
	}
	form := url.Values{
		"client_id": {g.env("GOOGLE_CLIENT_ID")}, "client_secret": {g.env("GOOGLE_CLIENT_SECRET")},
		"refresh_token": {refreshToken}, "grant_type": {"refresh_token"},
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := g.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var tokens struct {
		AccessToken      string `json:"access_token"`
		ExpiresIn        int    `json:"expires_in"`
		ErrorDescription string `json:"error_description"`
	}
	if json.NewDecoder(response.Body).Decode(&tokens) != nil || response.StatusCode < 200 || response.StatusCode >= 300 || tokens.AccessToken == "" {
		message := tokens.ErrorDescription
		if message == "" {
			message = "Token refresh failed"
		}
		_ = g.queries.MarkGoogleConnectionFailed(ctx, db.MarkGoogleConnectionFailedParams{LastError: &message, ID: connection.ID})
		return "", errors.New(message)
	}
	encrypted, err := g.cipher.Encrypt(tokens.AccessToken)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if tokens.ExpiresIn == 0 {
		expires = time.Now().Add(time.Hour)
	}
	if err := g.queries.UpdateGoogleConnectionTokens(ctx, db.UpdateGoogleConnectionTokensParams{AccessToken: &encrypted, TokenExpiresAt: &expires, ID: connection.ID}); err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

func (g *googleCalendarService) ensureAppCalendar(ctx context.Context, connection db.ConnectedApp, token string) (string, error) {
	if connection.AppCalendarID != nil && *connection.AppCalendarID != "" {
		return *connection.AppCalendarID, nil
	}
	body, _ := json.Marshal(map[string]string{"summary": "Connectient Appointments"})
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.googleapis.com/calendar/v3/calendars", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var calendar struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(response.Body).Decode(&calendar) != nil || response.StatusCode < 200 || response.StatusCode >= 300 || calendar.ID == "" {
		return "", fmt.Errorf("Google calendar creation returned %d", response.StatusCode)
	}
	if err := g.queries.SetGoogleAppCalendar(ctx, db.SetGoogleAppCalendarParams{AppCalendarID: &calendar.ID, ID: connection.ID}); err != nil {
		return "", err
	}
	return calendar.ID, nil
}

func (g *googleCalendarService) sendEventForCreate(ctx context.Context, calendarID, token string, event AppointmentEvent) (int, []byte, error) {
	body, _ := json.Marshal(googleEventPayload(event))
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, googleEventURL(calendarID, ""), bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, responseBody, err
}

func (g *googleCalendarService) sendEvent(ctx context.Context, method, calendarID, externalID, token string, event AppointmentEvent) (int, error) {
	body, _ := json.Marshal(googleEventPayload(event))
	request, _ := http.NewRequestWithContext(ctx, method, googleEventURL(calendarID, externalID), bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return 0, err
	}
	response.Body.Close()
	return response.StatusCode, nil
}

func googleEventPayload(event AppointmentEvent) map[string]any {
	start := event.ScheduledDate + "T" + trimCalendarTime(event.ScheduledTime) + ":00"
	clock, _ := time.Parse("15:04", trimCalendarTime(event.ScheduledTime))
	totalMinutes := clock.Hour()*60 + clock.Minute() + event.DurationMinutes
	if totalMinutes > 23*60 {
		totalMinutes = 23 * 60
	}
	end := fmt.Sprintf("%02d:%02d", totalMinutes/60, totalMinutes%60)
	return map[string]any{
		"summary":     event.PatientName + " — " + event.ProcedureType,
		"description": "Patient: " + event.PatientName + "\nPhone: " + event.Phone + "\nAppointment Type: " + event.ProcedureType,
		"start":       map[string]string{"dateTime": start, "timeZone": event.Timezone},
		"end":         map[string]string{"dateTime": event.ScheduledDate + "T" + end + ":00", "timeZone": event.Timezone},
	}
}

func googleEventURL(calendarID, eventID string) string {
	base := "https://www.googleapis.com/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events"
	if eventID != "" {
		return base + "/" + url.PathEscape(eventID)
	}
	return base
}

func (g *googleCalendarService) googleCalendarExists(ctx context.Context, calendarID, token string) (bool, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(calendarID), nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := g.client.Do(request)
	if err != nil {
		return false, err
	}
	response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("Google calendar check returned %d", response.StatusCode)
	}
	return true, nil
}

func (g *googleCalendarService) withPracticeLock(ctx context.Context, practiceID uuid.UUID, fn func() error) error {
	connection, err := g.db.Pool().Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	key := practiceID.String() + ":google-calendar"
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		return err
	}
	defer connection.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key)
	return fn()
}

func trimCalendarTime(value string) string {
	if len(value) >= 5 {
		return value[:5]
	}
	return value
}

func pointerInt32(value *int32, fallback int32) int32 {
	if value == nil {
		return fallback
	}
	return *value
}

func calendarStringPointer(value string) *string { return &value }

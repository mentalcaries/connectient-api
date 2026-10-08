//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestGoogleCalendarOAuthAndEventLifecycleIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newGoogleCalendarTestPool(t, ctx)
	practiceID, userID, appointmentID, unconfirmedID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Calendar Practice', 'Test City', 'calendar-practice', 'dental')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, first_name, last_name, email, practice_id, role, is_active)
		VALUES ($1, 'Practice', 'Owner', 'owner@example.test', $2, 'owner', TRUE)`, userID, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, appointment_type,
		 is_scheduled, is_confirmed, scheduled_date, scheduled_time, duration_minutes, scheduled_timezone)
		VALUES ($1, $2, 'Jane', 'Doe', 'jane@example.test', '+15550000000', 'consultation',
		 TRUE, TRUE, CURRENT_DATE + 1, '09:30', 45, 'America/Port_of_Spain')`, appointmentID, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO appointments
		(id, practice_id, first_name, last_name, email, mobile_phone, appointment_type,
		 is_scheduled, is_confirmed, scheduled_date, scheduled_time, duration_minutes, scheduled_timezone)
		VALUES ($1, $2, 'Not', 'Confirmed', 'pending@example.test', '+15550000001', 'consultation',
		 TRUE, FALSE, CURRENT_DATE + 1, '10:30', 30, 'America/Port_of_Spain')`, unconfirmedID, practiceID); err != nil {
		t.Fatal(err)
	}

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, _ := newTokenCipher(key)
	var mu sync.Mutex
	var requests []*http.Request
	var requestBodies []string
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body []byte
		if request.Body != nil {
			body, _ = io.ReadAll(request.Body)
		}
		mu.Lock()
		requests = append(requests, request)
		requestBodies = append(requestBodies, string(body))
		mu.Unlock()
		switch {
		case request.URL.Host == "oauth2.googleapis.com":
			return googleTestResponse(http.StatusOK, `{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3600}`), nil
		case request.URL.Path == "/oauth2/v2/userinfo":
			return googleTestResponse(http.StatusOK, `{"email":"calendar@example.test"}`), nil
		case request.Method == http.MethodPost && request.URL.Path == "/calendar/v3/calendars":
			return googleTestResponse(http.StatusOK, `{"id":"app-calendar@example.test"}`), nil
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/events"):
			return googleTestResponse(http.StatusOK, `{"id":"event-1"}`), nil
		case request.Method == http.MethodPatch:
			return googleTestResponse(http.StatusOK, `{}`), nil
		case request.Method == http.MethodDelete:
			return googleTestResponse(http.StatusNoContent, ``), nil
		default:
			return googleTestResponse(http.StatusNotFound, `{}`), nil
		}
	})
	queries := db.New(pool)
	calendar := &googleCalendarService{
		db: registrationTestDB{pool}, queries: queries, client: &http.Client{Transport: transport},
		env: func(keyName string) string {
			return map[string]string{
				"GOOGLE_CLIENT_ID": "client-id", "GOOGLE_CLIENT_SECRET": "client-secret",
				"GOOGLE_REDIRECT_URI": "https://api.example.test/connected-apps/google/callback",
				"FRONTEND_BASE_URL":   "https://app.example.test",
			}[keyName]
		},
		cipher: cipher,
	}
	s := &Server{db: registrationTestDB{pool}, DBQuery: queries, googleCalendar: calendar, appointmentEvents: calendar}
	owner := AuthUser{ID: userID, PracticeId: &practiceID}

	authResponse := serveGoogleAuth(t, ctx, s, owner)
	if authResponse.Code != http.StatusOK {
		t.Fatalf("auth=%d %s", authResponse.Code, authResponse.Body.String())
	}
	var state string
	for _, cookie := range authResponse.Result().Cookies() {
		if cookie.Name == "google_oauth_state" {
			state = cookie.Value
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/connected-apps/google/callback" {
				t.Errorf("unsafe state cookie: %+v", cookie)
			}
		}
	}
	if state == "" || !strings.Contains(authResponse.Body.String(), "calendar.app.created") || strings.Contains(authResponse.Body.String(), "auth/calendar ") {
		t.Fatalf("invalid auth response: %s state=%q", authResponse.Body.String(), state)
	}

	callback := serveGoogleCallback(ctx, s, state, "code-1", state)
	if callback.Code != http.StatusOK || !strings.Contains(callback.Body.String(), "GOOGLE_CALENDAR_CONNECTED") || !strings.Contains(callback.Body.String(), `"https://app.example.test"`) {
		t.Fatalf("callback=%d %s", callback.Code, callback.Body.String())
	}
	var encryptedAccess, encryptedRefresh *string
	if err := pool.QueryRow(ctx, `SELECT access_token, refresh_token FROM connected_apps WHERE practice_id=$1`, practiceID).Scan(&encryptedAccess, &encryptedRefresh); err != nil {
		t.Fatal(err)
	}
	if encryptedAccess == nil || encryptedRefresh == nil || *encryptedAccess == "access-1" || *encryptedRefresh == "refresh-1" {
		t.Fatal("OAuth tokens were not encrypted")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var confirmedMappings int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM appointment_calendar_events WHERE appointment_id=$1`, appointmentID).Scan(&confirmedMappings); err != nil {
			t.Fatal(err)
		}
		if confirmedMappings == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("confirmed future appointment was not backfilled")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var unconfirmedMappings int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM appointment_calendar_events WHERE appointment_id=$1`, unconfirmedID).Scan(&unconfirmedMappings); err != nil || unconfirmedMappings != 0 {
		t.Fatalf("unconfirmed appointment mappings=%d err=%v", unconfirmedMappings, err)
	}
	replay := serveGoogleCallback(ctx, s, state, "code-1", state)
	if !strings.Contains(replay.Body.String(), "GOOGLE_CALENDAR_ERROR") {
		t.Fatalf("state replay accepted: %s", replay.Body.String())
	}

	event := AppointmentEvent{
		AppointmentID: appointmentID, PracticeID: practiceID, PatientName: "Jane Doe",
		ProcedureType: "consultation", Phone: "+15550000000", ScheduledDate: "2026-10-08",
		ScheduledTime: "09:30", Timezone: "America/Port_of_Spain", DurationMinutes: 45,
	}
	mu.Lock()
	eventCreatesBefore := countGoogleEventCreates(requests)
	mu.Unlock()
	if err := calendar.SyncAppointmentCreated(ctx, event); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	eventCreatesAfter := countGoogleEventCreates(requests)
	mu.Unlock()
	if eventCreatesAfter != eventCreatesBefore {
		t.Fatalf("mapped appointment was duplicated: before=%d after=%d", eventCreatesBefore, eventCreatesAfter)
	}
	if err := calendar.SyncAppointmentUpdated(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := calendar.SyncAppointmentCancelled(ctx, practiceID, appointmentID); err != nil {
		t.Fatal(err)
	}
	for index, request := range requests {
		if strings.Contains(request.URL.Path, "/calendars/primary/") {
			t.Fatalf("request targeted primary calendar: %s", request.URL)
		}
		if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/events") && !strings.Contains(requestBodies[index], `"timeZone":"America/Port_of_Spain"`) {
			t.Fatalf("event omitted persisted timezone: %s", requestBodies[index])
		}
	}

	if _, err := pool.Exec(ctx, `INSERT INTO appointment_calendar_events (appointment_id, external_event_id) VALUES ($1, 'stale')`, appointmentID); err != nil {
		t.Fatal(err)
	}
	newEmail := "different-account@example.test"
	newAccess, _ := cipher.Encrypt("access-2")
	expiresAt := time.Now().Add(time.Hour)
	if err := s.saveGoogleConnection(ctx, db.UpsertGoogleConnectionParams{
		PracticeID: practiceID, ConnectedAccountEmail: &newEmail, AccessToken: &newAccess, TokenExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	var changedAccountMappings int
	var appCalendarID *string
	if err := pool.QueryRow(ctx, `SELECT app_calendar_id FROM connected_apps WHERE practice_id=$1`, practiceID).Scan(&appCalendarID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM appointment_calendar_events WHERE appointment_id=$1`, appointmentID).Scan(&changedAccountMappings); err != nil {
		t.Fatal(err)
	}
	if appCalendarID != nil || changedAccountMappings != 0 {
		t.Fatalf("account change retained calendar=%v mappings=%d", appCalendarID, changedAccountMappings)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO appointment_calendar_events (appointment_id, external_event_id) VALUES ($1, 'stale')`, appointmentID); err != nil {
		t.Fatal(err)
	}
	disconnect := serveGoogleDisconnect(ctx, s, owner)
	if disconnect.Code != http.StatusNoContent {
		t.Fatalf("disconnect=%d %s", disconnect.Code, disconnect.Body.String())
	}
	var connectionCount, mappingCount int
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM connected_apps WHERE practice_id=$1`, practiceID).Scan(&connectionCount)
	_ = pool.QueryRow(ctx, `SELECT COUNT(*) FROM appointment_calendar_events WHERE appointment_id=$1`, appointmentID).Scan(&mappingCount)
	if connectionCount != 0 || mappingCount != 0 {
		t.Fatalf("disconnect left connection=%d mappings=%d", connectionCount, mappingCount)
	}
}

func newGoogleCalendarTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	for _, file := range []string{
		"003_appointments.sql", "017_appointment_calendar_events.sql", "018_connected_apps.sql",
		"020_appointments_soft_delete.sql", "027_appointment_confirmation_state.sql",
		"031_appointment_staff_creation.sql", "033_google_calendar_sync.sql",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "db", "sql", "schema", file))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
	}
	return pool
}

func serveGoogleAuth(t *testing.T, ctx context.Context, s *Server, user AuthUser) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.GET("/connected-apps/google/auth", func(c *gin.Context) { c.Set("user", user); s.handlerGoogleCalendarAuth(c) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/connected-apps/google/auth", nil).WithContext(ctx))
	return response
}

func serveGoogleCallback(ctx context.Context, s *Server, state, code, cookieState string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/connected-apps/google/callback", s.handlerGoogleCalendarCallback)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/connected-apps/google/callback?state="+state+"&code="+code, nil).WithContext(ctx)
	request.AddCookie(&http.Cookie{Name: "google_oauth_state", Value: cookieState})
	router.ServeHTTP(response, request)
	return response
}

func serveGoogleDisconnect(ctx context.Context, s *Server, user AuthUser) *httptest.ResponseRecorder {
	router := gin.New()
	router.DELETE("/connected-apps/google", func(c *gin.Context) { c.Set("user", user); s.handlerDeleteGoogleCalendar(c) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/connected-apps/google", bytes.NewReader(nil)).WithContext(ctx))
	return response
}

func googleTestResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func countGoogleEventCreates(requests []*http.Request) int {
	count := 0
	for _, request := range requests {
		if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/events") {
			count++
		}
	}
	return count
}

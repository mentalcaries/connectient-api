package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

type ConnectedApp struct {
	Provider              string  `json:"provider"`
	ConnectedAccountEmail *string `json:"connected_account_email"`
	Connected             bool    `json:"connected"`
}

func (s *Server) handlerGetConnectedApps(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)

	dbConnectedApps, err := s.DBQuery.GetConnectedApps(c, *user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusBadRequest, "could not get connected apps", err)
		return
	}

	connectedApps := []ConnectedApp{}
	for _, dbConnectedApp := range dbConnectedApps {
		connectedApps = append(connectedApps, ConnectedApp{
			Provider:              dbConnectedApp.Provider,
			ConnectedAccountEmail: dbConnectedApp.ConnectedAccountEmail,
			Connected:             dbConnectedApp.IsConnected,
		})
	}

	c.JSON(http.StatusOK, connectedApps)
}

func (s *Server) handlerGoogleCalendarAuth(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	if s.googleCalendar == nil || s.googleCalendar.cipher == nil {
		respondWithError(c, http.StatusServiceUnavailable, "Google Calendar is not configured", nil)
		return
	}
	clientID, redirectURI := s.googleCalendar.env("GOOGLE_CLIENT_ID"), s.googleCalendar.env("GOOGLE_REDIRECT_URI")
	if clientID == "" || redirectURI == "" {
		respondWithError(c, http.StatusServiceUnavailable, "Google Calendar is not configured", nil)
		return
	}
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not begin Google authorization", err)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	if err := s.DBQuery.CreateGoogleOAuthState(c, db.CreateGoogleOAuthStateParams{
		State: state, PracticeID: *user.PracticeId, UserID: user.ID,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(10 * time.Minute), Valid: true},
	}); err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not begin Google authorization", err)
		return
	}
	secure := strings.HasPrefix(redirectURI, "https://")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("google_oauth_state", state, 600, googleCallbackCookiePath(redirectURI), "", secure, true)
	query := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"},
		"scope": {googleCalendarScope}, "access_type": {"offline"}, "prompt": {"consent"},
		"state": {state},
	}
	c.JSON(http.StatusOK, gin.H{"url": "https://accounts.google.com/o/oauth2/v2/auth?" + query.Encode()})
}

func (s *Server) handlerGoogleCalendarCallback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	state := c.Query("state")
	cookieState, cookieErr := c.Cookie("google_oauth_state")
	c.SetSameSite(http.SameSiteLaxMode)
	redirectURI := ""
	if s.googleCalendar != nil {
		redirectURI = s.googleCalendar.env("GOOGLE_REDIRECT_URI")
	}
	c.SetCookie("google_oauth_state", "", -1, googleCallbackCookiePath(redirectURI), "", strings.HasPrefix(redirectURI, "https://"), true)
	if state == "" || cookieErr != nil || cookieState != state {
		s.googleCalendarPopup(c, false, "Invalid or expired authorization state")
		return
	}
	bound, err := s.DBQuery.ConsumeGoogleOAuthState(c, state)
	if errors.Is(err, pgx.ErrNoRows) {
		s.googleCalendarPopup(c, false, "Invalid or expired authorization state")
		return
	}
	if err != nil || s.googleCalendar == nil || s.googleCalendar.cipher == nil {
		s.googleCalendarPopup(c, false, "Google Calendar is unavailable")
		return
	}
	user, err := s.DBQuery.GetUserAuthorization(c, bound.UserID)
	if err != nil || !membershipAllowed(user) || user.Role == nil || *user.Role != "owner" || user.PracticeID == nil || *user.PracticeID != bound.PracticeID {
		s.googleCalendarPopup(c, false, "Google Calendar access is no longer authorized")
		return
	}
	if oauthError := c.Query("error"); oauthError != "" {
		s.googleCalendarPopup(c, false, "Google authorization was cancelled")
		return
	}
	if c.Query("code") == "" {
		s.googleCalendarPopup(c, false, "Google did not provide an authorization code")
		return
	}
	tokens, err := s.exchangeGoogleAuthorizationCode(c, c.Query("code"))
	if err != nil {
		s.googleCalendarPopup(c, false, "Could not connect Google Calendar")
		return
	}
	email, err := s.googleAccountEmail(c, tokens.AccessToken)
	if err != nil {
		s.googleCalendarPopup(c, false, "Could not read the Google account")
		return
	}
	encryptedAccess, err := s.googleCalendar.cipher.Encrypt(tokens.AccessToken)
	if err != nil {
		s.googleCalendarPopup(c, false, "Could not secure Google credentials")
		return
	}
	var encryptedRefresh *string
	if tokens.RefreshToken != "" {
		value, encryptErr := s.googleCalendar.cipher.Encrypt(tokens.RefreshToken)
		if encryptErr != nil {
			s.googleCalendarPopup(c, false, "Could not secure Google credentials")
			return
		}
		encryptedRefresh = &value
	}
	expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	if tokens.ExpiresIn <= 0 {
		expiresAt = time.Now().Add(time.Hour)
	}
	if err := s.saveGoogleConnection(c, db.UpsertGoogleConnectionParams{
		PracticeID: bound.PracticeID, ConnectedAccountEmail: &email, AccessToken: &encryptedAccess,
		RefreshToken: encryptedRefresh, TokenExpiresAt: &expiresAt,
	}); err != nil {
		s.googleCalendarPopup(c, false, "Could not save Google Calendar connection")
		return
	}
	go func(practiceID uuid.UUID) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.googleCalendar.Backfill(ctx, practiceID); err != nil {
			fmt.Printf("Google Calendar backfill failed for %s: %v\n", practiceID, err)
		}
	}(bound.PracticeID)
	s.googleCalendarPopup(c, true, "")
}

func (s *Server) saveGoogleConnection(ctx context.Context, params db.UpsertGoogleConnectionParams) error {
	tx, err := s.db.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	queries := s.DBQuery.WithTx(tx)
	existing, err := queries.GetGoogleConnectionAny(ctx, params.PracticeID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && existing.ConnectedAccountEmail != nil && params.ConnectedAccountEmail != nil &&
		!strings.EqualFold(*existing.ConnectedAccountEmail, *params.ConnectedAccountEmail) {
		if err := queries.DeleteGoogleCalendarMappingsForPractice(ctx, params.PracticeID); err != nil {
			return err
		}
	}
	if _, err := queries.UpsertGoogleConnection(ctx, params); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type googleOAuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func (s *Server) exchangeGoogleAuthorizationCode(ctx context.Context, code string) (googleOAuthTokens, error) {
	form := url.Values{
		"code": {code}, "client_id": {s.googleCalendar.env("GOOGLE_CLIENT_ID")},
		"client_secret": {s.googleCalendar.env("GOOGLE_CLIENT_SECRET")}, "redirect_uri": {s.googleCalendar.env("GOOGLE_REDIRECT_URI")},
		"grant_type": {"authorization_code"},
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.googleCalendar.client.Do(request)
	if err != nil {
		return googleOAuthTokens{}, err
	}
	defer response.Body.Close()
	var tokens googleOAuthTokens
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || tokens.AccessToken == "" {
		return googleOAuthTokens{}, errors.New("Google token exchange failed")
	}
	return tokens, nil
}

func (s *Server) googleAccountEmail(ctx context.Context, accessToken string) (string, error) {
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := s.googleCalendar.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var profile struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil || response.StatusCode < 200 || response.StatusCode >= 300 || profile.Email == "" {
		return "", errors.New("Google user info failed")
	}
	return profile.Email, nil
}

func (s *Server) handlerDeleteGoogleCalendar(c *gin.Context) {
	setPrivateNoStore(c)
	user := c.MustGet("user").(AuthUser)
	tx, err := s.db.Pool().Begin(c)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not disconnect Google Calendar", err)
		return
	}
	defer tx.Rollback(c)
	queries := s.DBQuery.WithTx(tx)
	if err := queries.DeleteGoogleCalendarMappingsForPractice(c, *user.PracticeId); err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not disconnect Google Calendar", err)
		return
	}
	if err := queries.DeleteGoogleConnection(c, *user.PracticeId); err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not disconnect Google Calendar", err)
		return
	}
	if err := tx.Commit(c); err != nil {
		respondWithError(c, http.StatusInternalServerError, "Could not disconnect Google Calendar", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) googleCalendarPopup(c *gin.Context, success bool, message string) {
	origin := "null"
	frontendBaseURL := os.Getenv("FRONTEND_BASE_URL")
	if s.googleCalendar != nil {
		frontendBaseURL = s.googleCalendar.env("FRONTEND_BASE_URL")
	}
	if parsed, err := url.Parse(frontendBaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	payload := map[string]string{"type": "GOOGLE_CALENDAR_CONNECTED"}
	if !success {
		payload = map[string]string{"type": "GOOGLE_CALENDAR_ERROR", "error": message}
	}
	encodedPayload, _ := json.Marshal(payload)
	encodedOrigin, _ := json.Marshal(origin)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<!doctype html><html><body><script>window.opener && window.opener.postMessage("+string(encodedPayload)+","+string(encodedOrigin)+");window.close();</script></body></html>"))
}

func googleCallbackCookiePath(redirectURI string) string {
	if parsed, err := url.Parse(redirectURI); err == nil && strings.HasPrefix(parsed.Path, "/") && parsed.Path != "" {
		return parsed.Path
	}
	return "/connected-apps/google/callback"
}

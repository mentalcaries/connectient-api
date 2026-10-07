//go:build integration

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestAccountIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	practiceID, userID, otherUserID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO practices (id, name, city, practice_code, practice_category)
		VALUES ($1, 'Account Practice', 'Test City', 'account-practice', 'dental')`, practiceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users
		(id, first_name, last_name, email, mobile_phone, practice_id, role)
		VALUES ($1, 'Account', 'User', 'account@example.test', '+15555550101', $3, 'staff'),
		       ($2, 'Other', 'User', 'other@example.test', '+15555550102', $3, 'staff')`, userID, otherUserID, practiceID); err != nil {
		t.Fatal(err)
	}
	storage := &fakeObjectStorage{publicBase: "https://media.example.test", objects: map[string][]byte{}}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool), storage: storage}
	role := "staff"
	user := AuthUser{ID: userID, PracticeId: &practiceID, Role: &role}

	response := requestAccount(ctx, s, http.MethodGet, "/account", user, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"first_name":"Account"`) {
		t.Fatalf("account GET = %d %s", response.Code, response.Body.String())
	}
	avatarURL := storage.PublicURL(practiceID.String() + "/avatars/" + userID.String())
	response = requestAccount(ctx, s, http.MethodPatch, "/account", user, map[string]any{
		"first_name": " Updated ", "last_name": " Person ", "avatar_url": avatarURL,
		"mobile_phone": "+15555550103", "whatsapp_notifications_enabled": false,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("account PATCH = %d %s", response.Code, response.Body.String())
	}
	var firstName, lastName string
	var mobile, avatar *string
	var whatsapp bool
	if err := pool.QueryRow(ctx, `SELECT first_name, last_name, mobile_phone, avatar_url, whatsapp_notifications_enabled FROM users WHERE id = $1`, userID).
		Scan(&firstName, &lastName, &mobile, &avatar, &whatsapp); err != nil {
		t.Fatal(err)
	}
	if firstName != "Updated" || lastName != "Person" || mobile == nil || *mobile != "+15555550103" || avatar == nil || *avatar != avatarURL || whatsapp {
		t.Errorf("unexpected account persistence: %q %q mobile=%v avatar=%v whatsapp=%t", firstName, lastName, mobile, avatar, whatsapp)
	}
	var otherFirstName string
	if err := pool.QueryRow(ctx, `SELECT first_name FROM users WHERE id = $1`, otherUserID).Scan(&otherFirstName); err != nil || otherFirstName != "Other" {
		t.Errorf("other account changed: name=%q err=%v", otherFirstName, err)
	}

	response = requestAccount(ctx, s, http.MethodPatch, "/account", user, map[string]any{
		"first_name": "Updated", "last_name": "Person", "avatar_url": "https://attacker.example/avatar",
	})
	if response.Code != http.StatusBadRequest {
		t.Errorf("external avatar status = %d, want 400", response.Code)
	}
	response = requestAccount(ctx, s, http.MethodPatch, "/account", user, map[string]any{
		"first_name": "Updated", "last_name": "Person", "mobile_phone": "+15555550102",
	})
	if response.Code != http.StatusConflict {
		t.Errorf("duplicate mobile status = %d, want 409", response.Code)
	}

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 504)...)
	response = requestAvatarUpload(ctx, s, user, png, "image/png")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), avatarURL) {
		t.Fatalf("avatar upload = %d %s", response.Code, response.Body.String())
	}
	key := practiceID.String() + "/avatars/" + userID.String()
	if !bytes.Equal(storage.objects[key], png) {
		t.Error("avatar was not stored at the fixed key")
	}
	response = requestAccount(ctx, s, http.MethodDelete, "/upload/avatar", user, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("avatar delete = %d %s", response.Code, response.Body.String())
	}
	if _, exists := storage.objects[key]; exists {
		t.Error("avatar fixed key was not deleted")
	}
}

func requestAccount(ctx context.Context, s *Server, method, path string, user AuthUser, body any) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	switch {
	case method == http.MethodGet:
		router.GET(path, setUser, s.handlerGetAccount)
	case method == http.MethodPatch:
		router.PATCH(path, setUser, s.handlerPatchAccount)
	case method == http.MethodDelete:
		router.DELETE(path, setUser, s.handlerDeleteAvatar)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func requestAvatarUpload(ctx context.Context, s *Server, user AuthUser, data []byte, contentType string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="avatar.png"`)
	header.Set("Content-Type", contentType)
	part, _ := writer.CreatePart(header)
	_, _ = part.Write(data)
	_ = writer.Close()
	router := gin.New()
	router.POST("/upload/avatar", func(c *gin.Context) { c.Set("user", user) }, s.handlerUploadAvatar)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/upload/avatar", &body).WithContext(ctx)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(response, request)
	return response
}

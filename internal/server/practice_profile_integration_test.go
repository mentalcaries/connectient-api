//go:build integration

package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestPracticeProfileIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	oldLogo := fmt.Sprintf("https://media.example.test/%s/logo_old", practiceID)
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, email, practice_code, practice_category, logo)
		VALUES ($1, 'Profile Practice', 'Test City', 'profile@example.test', 'profile-practice', 'dental', $2),
		       ($3, 'Other Practice', 'Test City', 'other@example.test', 'taken-profile-code', 'dental', NULL)`,
		practiceID, oldLogo, otherPracticeID); err != nil {
		t.Fatal(err)
	}
	storage := &fakeObjectStorage{publicBase: "https://media.example.test", objects: map[string][]byte{practiceID.String() + "/logo_old": {1}}}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool), storage: storage}
	owner, admin, staff := "owner", "admin", "staff"

	response := requestPracticeProfileGet(ctx, s, AuthUser{PracticeId: &practiceID, Role: &staff})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"practice_code":"profile-practice"`) {
		t.Fatalf("profile GET = %d %s", response.Code, response.Body.String())
	}
	response = requestPracticeCodePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &admin}, `{"practiceCode":"profile-practice"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("same code response = %d %s", response.Code, response.Body.String())
	}
	response = requestPracticeCodePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, `{"practiceCode":"taken-profile-code"}`)
	if response.Code != http.StatusConflict {
		t.Errorf("taken code status = %d, want 409", response.Code)
	}
	response = requestPracticeCodePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, `{"practiceCode":"admin"}`)
	if response.Code != http.StatusConflict {
		t.Errorf("reserved code status = %d, want 409", response.Code)
	}

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 504)...)
	storage.onDelete = func(key string) {
		if key == practiceID.String()+"/logo_old" {
			var current *string
			if err := pool.QueryRow(ctx, `SELECT logo FROM practices WHERE id = $1`, practiceID).Scan(&current); err != nil || current == nil || *current == oldLogo {
				t.Errorf("old logo deleted before database replacement: current=%v err=%v", current, err)
			}
		}
	}
	response = requestPracticeProfilePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, validProfileFields("updated-profile-code"), png)
	if response.Code != http.StatusOK {
		t.Fatalf("profile upload response = %d %s", response.Code, response.Body.String())
	}
	var currentLogo *string
	var name, streetAddress string
	var multiple bool
	if err := pool.QueryRow(ctx, `SELECT name, street_address, has_multiple_providers, logo FROM practices WHERE id = $1`, practiceID).
		Scan(&name, &streetAddress, &multiple, &currentLogo); err != nil {
		t.Fatal(err)
	}
	if name != "Updated Practice" || streetAddress != "" || !multiple || currentLogo == nil || *currentLogo == oldLogo {
		t.Errorf("unexpected profile persistence: name=%q street=%q multiple=%t logo=%v", name, streetAddress, multiple, currentLogo)
	}
	if _, exists := storage.objects[practiceID.String()+"/logo_old"]; exists {
		t.Error("old owned logo was not deleted")
	}

	newKey, ok := storage.OwnedKey(*currentLogo, practiceID.String())
	if !ok {
		t.Fatalf("new logo is not owned: %s", *currentLogo)
	}
	fields := validProfileFields("taken-profile-code")
	response = requestPracticeProfilePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &owner}, fields, png)
	if response.Code != http.StatusConflict {
		t.Fatalf("profile duplicate response = %d %s", response.Code, response.Body.String())
	}
	if len(storage.objects) != 1 || storage.objects[newKey] == nil {
		t.Errorf("failed replacement object was not cleaned up: keys=%v", storage.keys())
	}

	storage.onDelete = func(key string) {
		if key == newKey {
			var current *string
			if err := pool.QueryRow(ctx, `SELECT logo FROM practices WHERE id = $1`, practiceID).Scan(&current); err != nil || current != nil {
				t.Errorf("logo object deleted before database clear: current=%v err=%v", current, err)
			}
		}
	}
	removeFields := validProfileFields("updated-profile-code")
	removeFields["logoAction"] = "remove"
	response = requestPracticeProfilePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &admin}, removeFields, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"newLogoUrl":null`) {
		t.Fatalf("profile remove response = %d %s", response.Code, response.Body.String())
	}
	if _, exists := storage.objects[newKey]; exists {
		t.Error("removed logo object still exists")
	}
	response = requestPracticeProfilePatch(ctx, s, AuthUser{PracticeId: &practiceID, Role: &staff}, validProfileFields("updated-profile-code"), nil)
	if response.Code != http.StatusForbidden {
		t.Errorf("staff profile status = %d, want 403", response.Code)
	}
}

type fakeObjectStorage struct {
	publicBase string
	objects    map[string][]byte
	onDelete   func(string)
}

func (storage *fakeObjectStorage) Put(_ context.Context, key, _ string, body io.Reader, _ int64) error {
	data, err := io.ReadAll(body)
	if err == nil {
		storage.objects[key] = data
	}
	return err
}

func (storage *fakeObjectStorage) Delete(_ context.Context, key string) error {
	if storage.onDelete != nil {
		storage.onDelete(key)
	}
	delete(storage.objects, key)
	return nil
}

func (storage *fakeObjectStorage) PublicURL(key string) string { return storage.publicBase + "/" + key }

func (storage *fakeObjectStorage) OwnedKey(rawURL, practiceID string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host != "media.example.test" {
		return "", false
	}
	key := strings.TrimPrefix(parsed.Path, "/")
	return key, strings.HasPrefix(key, practiceID+"/")
}

func (storage *fakeObjectStorage) keys() []string {
	keys := make([]string, 0, len(storage.objects))
	for key := range storage.objects {
		keys = append(keys, key)
	}
	return keys
}

func validProfileFields(code string) map[string]string {
	return map[string]string{
		"name": "Updated Practice", "city": "Test City", "email": "updated@example.test",
		"practiceCode": code, "has_multiple_providers": "true",
	}
}

func requestPracticeProfileGet(ctx context.Context, s *Server, user AuthUser) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/practices/profile", func(c *gin.Context) { c.Set("user", user) }, s.handlerGetPracticeProfile)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/practices/profile", nil).WithContext(ctx))
	return response
}

func requestPracticeCodePatch(ctx context.Context, s *Server, user AuthUser, body string) *httptest.ResponseRecorder {
	router := gin.New()
	router.PATCH("/practices/practice-code", func(c *gin.Context) { c.Set("user", user) }, requireAdmin(), s.handlerPatchPracticeCode)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/practices/practice-code", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func requestPracticeProfilePatch(ctx context.Context, s *Server, user AuthUser, fields map[string]string, logo []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		_ = writer.WriteField(name, value)
	}
	if logo != nil {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="logoFile"; filename="logo.png"`)
		header.Set("Content-Type", "image/png")
		part, _ := writer.CreatePart(header)
		_, _ = part.Write(logo)
		if _, exists := fields["logoAction"]; !exists {
			_ = writer.WriteField("logoAction", "upload")
		}
	}
	_ = writer.Close()
	router := gin.New()
	router.PATCH("/practices/profile", func(c *gin.Context) { c.Set("user", user) }, requireAdmin(), s.handlerPatchPracticeProfile)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/practices/profile", &body).WithContext(ctx)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	router.ServeHTTP(response, request)
	return response
}

//go:build integration

package server

import (
	"bytes"
	"context"
	"fmt"
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

func TestPracticeLogoIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pool := newRegistrationTestPool(t, ctx, registrationTestConfig(t))
	practiceID, failurePracticeID := uuid.New(), uuid.New()
	oldKey := practiceID.String() + "/legacy_logo"
	oldURL := "https://media.example.test/" + oldKey
	if _, err := pool.Exec(ctx, `INSERT INTO practices
		(id, name, city, practice_code, practice_category, logo)
		VALUES ($1, 'Logo Practice', 'Test City', 'logo-practice', 'dental', $2),
		       ($3, 'Failure Practice', 'Test City', 'logo-failure', 'dental', NULL)`, practiceID, oldURL, failurePracticeID); err != nil {
		t.Fatal(err)
	}
	storage := &fakeObjectStorage{publicBase: "https://media.example.test", objects: map[string][]byte{oldKey: {1}}}
	s := &Server{db: registrationTestDB{pool}, DBQuery: db.New(pool), storage: storage}
	owner, staff := "owner", "staff"
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 504)...)

	storage.onDelete = func(key string) {
		if key == oldKey {
			var current *string
			if err := pool.QueryRow(ctx, `SELECT logo FROM practices WHERE id = $1`, practiceID).Scan(&current); err != nil || current == nil || *current == oldURL {
				t.Errorf("old logo deleted before database replacement: current=%v err=%v", current, err)
			}
		}
	}
	response := requestPracticeLogo(ctx, s, http.MethodPost, AuthUser{PracticeId: &practiceID, Role: &owner}, png)
	if response.Code != http.StatusOK {
		t.Fatalf("logo upload = %d %s", response.Code, response.Body.String())
	}
	var logoURL *string
	if err := pool.QueryRow(ctx, `SELECT logo FROM practices WHERE id = $1`, practiceID).Scan(&logoURL); err != nil || logoURL == nil || !strings.Contains(*logoURL, practiceID.String()+"/logo_") {
		t.Fatalf("unexpected persisted logo: %v err=%v", logoURL, err)
	}
	newKey, _ := storage.OwnedKey(*logoURL, practiceID.String())
	if _, exists := storage.objects[oldKey]; exists {
		t.Error("old logo object was not removed")
	}

	storage.onDelete = func(key string) {
		if key == newKey {
			var current *string
			if err := pool.QueryRow(ctx, `SELECT logo FROM practices WHERE id = $1`, practiceID).Scan(&current); err != nil || current != nil {
				t.Errorf("logo deleted before database clear: current=%v err=%v", current, err)
			}
		}
	}
	response = requestPracticeLogo(ctx, s, http.MethodDelete, AuthUser{PracticeId: &practiceID, Role: &owner}, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("logo delete = %d %s", response.Code, response.Body.String())
	}
	if _, exists := storage.objects[newKey]; exists {
		t.Error("deleted logo object remains")
	}
	response = requestPracticeLogo(ctx, s, http.MethodPost, AuthUser{PracticeId: &practiceID, Role: &staff}, png)
	if response.Code != http.StatusForbidden {
		t.Errorf("staff upload status = %d, want 403", response.Code)
	}

	constraint := fmt.Sprintf(`ALTER TABLE practices ADD CONSTRAINT injected_logo_failure CHECK (id <> '%s'::uuid OR logo IS NULL)`, failurePracticeID)
	if _, err := pool.Exec(ctx, constraint); err != nil {
		t.Fatal(err)
	}
	before := len(storage.objects)
	storage.onDelete = nil
	response = requestPracticeLogo(ctx, s, http.MethodPost, AuthUser{PracticeId: &failurePracticeID, Role: &owner}, png)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("failed update response = %d %s", response.Code, response.Body.String())
	}
	if len(storage.objects) != before {
		t.Errorf("failed upload was not cleaned up: keys=%v", storage.keys())
	}
}

func requestPracticeLogo(ctx context.Context, s *Server, method string, user AuthUser, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	contentType := ""
	if method == http.MethodPost {
		writer := multipart.NewWriter(&body)
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="logo.png"`)
		header.Set("Content-Type", "image/png")
		part, _ := writer.CreatePart(header)
		_, _ = part.Write(data)
		_ = writer.Close()
		contentType = writer.FormDataContentType()
	}
	router := gin.New()
	setUser := func(c *gin.Context) { c.Set("user", user) }
	if method == http.MethodPost {
		router.POST("/upload/logo", setUser, requireAdmin(), s.handlerUploadPracticeLogo)
	} else {
		router.DELETE("/upload/logo", setUser, requireAdmin(), s.handlerDeletePracticeLogo)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/upload/logo", &body).WithContext(ctx)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	router.ServeHTTP(response, request)
	return response
}

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrIdentityProfileUnavailable = errors.New("identity profile service unavailable")

type IdentityProfileService interface {
	UpdateDisplayName(context.Context, uuid.UUID, string) error
}

type httpIdentityProfileService struct {
	url    string
	token  string
	client *http.Client
}

func newHTTPIdentityProfileServiceFromEnv() IdentityProfileService {
	url := strings.TrimSpace(os.Getenv("AUTH_PROFILE_SERVICE_URL"))
	token := strings.TrimSpace(os.Getenv("AUTH_PROFILE_SERVICE_TOKEN"))
	if url == "" || token == "" {
		return nil
	}
	return &httpIdentityProfileService{
		url: url, token: token, client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *httpIdentityProfileService) UpdateDisplayName(ctx context.Context, userID uuid.UUID, name string) error {
	payload, err := json.Marshal(map[string]string{"user_id": userID.String(), "name": name})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+s.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("update identity display name: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("update identity display name: status %d", response.StatusCode)
	}
	return nil
}

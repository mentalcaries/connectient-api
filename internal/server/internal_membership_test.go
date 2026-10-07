package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestInternalMembershipLookupRejectsInvalidServiceRequests(t *testing.T) {
	t.Setenv("AUTH_PROFILE_SERVICE_TOKEN", "fixture-service-token")
	s := &Server{}
	router := gin.New()
	router.POST("/internal/auth/membership", s.handlerInternalMembershipLookup)

	request := httptest.NewRequest(http.MethodPost, "/internal/auth/membership", strings.NewReader(`{"userId":"not-a-uuid"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token = %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/internal/auth/membership", strings.NewReader(`{"userId":"not-a-uuid"}`))
	request.Header.Set("Authorization", "Bearer fixture-service-token")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid body = %d %s", response.Code, response.Body.String())
	}
}

func TestDeleteTestMembershipIsUnavailableOutsideE2E(t *testing.T) {
	t.Setenv("E2E_TEST_MODE", "false")
	t.Setenv("AUTH_PROFILE_SERVICE_TOKEN", "fixture-service-token")
	s := &Server{}
	router := gin.New()
	router.DELETE("/internal/test/memberships/:id", s.handlerDeleteTestMembership)
	request := httptest.NewRequest(http.MethodDelete, "/internal/test/memberships/"+uuid.NewString(), nil)
	request.Header.Set("Authorization", "Bearer fixture-service-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("non-E2E fixture = %d %s", response.Code, response.Body.String())
	}
}

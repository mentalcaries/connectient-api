package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mentalcaries/connectient-api/internal/database"
)

type healthTestDatabase struct {
	status database.HealthStatus
}

func (fixture healthTestDatabase) Health() database.HealthStatus {
	return fixture.status
}

func (healthTestDatabase) Pool() *pgxpool.Pool { return nil }
func (healthTestDatabase) Close() error        { return nil }

func TestHealthHandlerStatus(t *testing.T) {
	tests := []struct {
		name       string
		health     database.HealthStatus
		wantStatus int
	}{
		{name: "healthy", health: database.HealthStatus{Status: "up"}, wantStatus: http.StatusOK},
		{name: "database down", health: database.HealthStatus{Status: "down", Error: "sensitive database details"}, wantStatus: http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{db: healthTestDatabase{status: test.health}}
			router := gin.New()
			router.GET("/health", server.healthHandler)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

			if response.Code != test.wantStatus {
				t.Fatalf("health status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if response.Body.String() != `{"status":"`+test.health.Status+`"}` {
				t.Fatalf("unexpected public health body: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), test.health.Error) && test.health.Error != "" {
				t.Fatal("public health body exposed database error details")
			}
		})
	}
}

package server

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAppointmentPatchRoute(t *testing.T) {
	s := &Server{}
	router := s.RegisterRoutes().(*gin.Engine)
	found := false
	for _, route := range router.Routes() {
		if route.Method != http.MethodPatch {
			continue
		}
		if route.Path == "/appointments" {
			t.Error("PATCH must include the appointment ID in its path")
		}
		if route.Path == "/appointments/:id" {
			found = true
		}
	}
	if !found {
		t.Fatal("PATCH /appointments/:id is not registered")
	}
}

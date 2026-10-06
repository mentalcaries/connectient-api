package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

func TestMembershipAccess(t *testing.T) {
	practiceID, deletedAt := uuid.New(), time.Now()
	owner, admin, staff, unknown := "owner", "admin", "staff", "unknown"
	for _, tc := range []struct {
		name                                         string
		role                                         *string
		active, deleted, suspended, noPractice, want bool
	}{
		{name: "owner", role: &owner, active: true, want: true},
		{name: "admin", role: &admin, active: true, want: true},
		{name: "staff", role: &staff, active: true, want: true},
		{name: "inactive", role: &owner},
		{name: "deleted", role: &owner, active: true, deleted: true},
		{name: "suspended", role: &owner, active: true, suspended: true},
		{name: "no practice", role: &owner, active: true, noPractice: true},
		{name: "no role", active: true},
		{name: "unknown role", role: &unknown, active: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := db.GetUserAuthorizationRow{PracticeID: &practiceID, Role: tc.role, IsActive: tc.active, IsSuspended: tc.suspended}
			if tc.deleted {
				user.DeletedAt = &deletedAt
			}
			if tc.noPractice {
				user.PracticeID = nil
			}
			if got := membershipAllowed(user); got != tc.want {
				t.Errorf("allowed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOwnerAccess(t *testing.T) {
	for _, role := range []string{"owner", "admin", "staff", ""} {
		t.Run("role="+role, func(t *testing.T) {
			router := gin.New()
			router.GET("/", func(c *gin.Context) {
				user := AuthUser{}
				if role != "" {
					user.Role = &role
				}
				c.Set("user", user)
			}, requireOwner(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			want := http.StatusForbidden
			if role == "owner" {
				want = http.StatusNoContent
			}
			if response.Code != want {
				t.Errorf("status = %d, want %d", response.Code, want)
			}
		})
	}
}

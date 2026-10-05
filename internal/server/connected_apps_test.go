package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	db "github.com/mentalcaries/connectient-api/internal/database"
)

// Embedding DBTX leaves unexpected database operations unsupported; this handler
// should only execute its list query.
type connectedAppsTestDB struct {
	db.DBTX
	query func(context.Context, string, ...any) (pgx.Rows, error)
}

func (d connectedAppsTestDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return d.query(ctx, sql, args...)
}

type connectedAppsTestRows struct {
	pgx.Rows
	items  []db.GetConnectedAppsRow
	next   int
	closed bool
}

func (r *connectedAppsTestRows) Next() bool { return r.next < len(r.items) }
func (r *connectedAppsTestRows) Close()     { r.closed = true }
func (r *connectedAppsTestRows) Err() error { return nil }
func (r *connectedAppsTestRows) Scan(dest ...any) error {
	if len(dest) != 3 {
		return errors.New("connected-app list must scan only its three public fields")
	}
	item := r.items[r.next]
	*dest[0].(*string) = item.Provider
	*dest[1].(**string) = item.ConnectedAccountEmail
	*dest[2].(*bool) = item.IsConnected
	r.next++
	return nil
}

func TestConnectedAppsResponse(t *testing.T) {
	email := "calendar@example.test"
	for _, tc := range []struct {
		name     string
		items    []db.GetConnectedAppsRow
		queryErr error
		status   int
		body     string
	}{
		{
			name:   "connected account exposes only public fields",
			items:  []db.GetConnectedAppsRow{{Provider: "google", ConnectedAccountEmail: &email, IsConnected: true}},
			status: http.StatusOK,
			body:   `[{"provider":"google","connected_account_email":"calendar@example.test","connected":true}]`,
		},
		{
			name:   "disconnected account preserves null email",
			items:  []db.GetConnectedAppsRow{{Provider: "google", IsConnected: false}},
			status: http.StatusOK,
			body:   `[{"provider":"google","connected_account_email":null,"connected":false}]`,
		},
		{name: "empty list", status: http.StatusOK, body: `[]`},
		{
			name:     "query failure returns only an error",
			queryErr: errors.New("database unavailable"),
			status:   http.StatusBadRequest,
			body:     `{"error":"could not get connected apps"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			practiceID := uuid.New()
			rows := &connectedAppsTestRows{items: tc.items}
			calls := 0
			store := connectedAppsTestDB{query: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
				calls++
				if len(args) != 1 || args[0] != practiceID {
					t.Fatalf("query must use the authenticated practice ID, got %v", args)
				}
				// Guard against fetching secret columns even if the DTO omits them.
				for _, forbidden := range []string{"*", "access_token", "refresh_token", "token_expires_at", "last_error"} {
					if strings.Contains(strings.ToLower(sql), forbidden) {
						t.Errorf("list query requests non-public data: %s", forbidden)
					}
				}
				return rows, tc.queryErr
			}}
			s := &Server{DBQuery: db.New(store)}
			router := gin.New()
			router.GET("/practices/connected-apps", func(c *gin.Context) {
				c.Set("user", AuthUser{PracticeId: &practiceID})
				s.handlerGetConnectedApps(c)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/practices/connected-apps", nil))
			if response.Code != tc.status {
				t.Errorf("status = %d, want %d", response.Code, tc.status)
			}
			if response.Body.String() != tc.body {
				t.Errorf("body = %s, want %s", response.Body.String(), tc.body)
			}
			if calls != 1 {
				t.Errorf("query calls = %d, want 1", calls)
			}
			if tc.queryErr == nil && !rows.closed {
				t.Error("query rows were not closed")
			}
		})
	}
}

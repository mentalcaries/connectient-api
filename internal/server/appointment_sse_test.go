package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestAppointmentEventHubScopesAndCoalescesInvalidations(t *testing.T) {
	hub := newAppointmentEventHub()
	practiceID, otherPracticeID := uuid.New(), uuid.New()
	events, unsubscribe, err := hub.subscribe(practiceID)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	otherEvents, otherUnsubscribe, err := hub.subscribe(otherPracticeID)
	if err != nil {
		t.Fatal(err)
	}
	defer otherUnsubscribe()

	firstID, latestID := uuid.New(), uuid.New()
	if err := hub.BroadcastAppointmentChange(context.Background(), practiceID, "INSERT", firstID); err != nil {
		t.Fatal(err)
	}
	if err := hub.BroadcastAppointmentChange(context.Background(), practiceID, "UPDATE", latestID); err != nil {
		t.Fatal(err)
	}
	if event := <-events; event.Event != "UPDATE" || event.ID != latestID || event.PracticeID != practiceID {
		t.Fatalf("unexpected coalesced event: %+v", event)
	}
	select {
	case event := <-otherEvents:
		t.Fatalf("cross-practice event: %+v", event)
	default:
	}
	if err := hub.BroadcastAppointmentChange(context.Background(), practiceID, "UPSERT", uuid.New()); err == nil {
		t.Fatal("expected invalid event to fail")
	}
}

func TestAppointmentEventsStreamsAuthenticatedPractice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := newAppointmentEventHub()
	practiceID, appointmentID := uuid.New(), uuid.New()
	user := AuthUser{PracticeId: &practiceID}
	s := &Server{appointmentEventHub: hub}
	router := gin.New()
	router.GET("/appointments/events", func(c *gin.Context) { c.Set("user", user) }, s.handlerAppointmentEvents)
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 25 * time.Millisecond
	server.Start()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/appointments/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("stream headers = %d %+v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	connected, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(connected) != ": connected" {
		t.Fatalf("initial stream line = %q err=%v", connected, err)
	}
	_, _ = reader.ReadString('\n')

	// The production server has a normal request WriteTimeout. The SSE handler
	// clears that connection deadline before its first write so streaming can
	// continue beyond it.
	time.Sleep(50 * time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for hub.subscriberCount(practiceID) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := hub.BroadcastAppointmentChange(context.Background(), practiceID, "DELETE", appointmentID); err != nil {
		t.Fatal(err)
	}
	eventLine, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(eventLine) != "event: DELETE" {
		t.Fatalf("event line = %q err=%v", eventLine, err)
	}
	dataLine, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(dataLine, appointmentID.String()) || !strings.Contains(dataLine, practiceID.String()) {
		t.Fatalf("data line = %q err=%v", dataLine, err)
	}
	cancel()
}

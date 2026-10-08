package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	appointmentSSEHeartbeat      = 15 * time.Second
	appointmentSSEMaxLifetime    = 5 * time.Minute
	appointmentSSEMaxSubscribers = 100
)

type appointmentInvalidation struct {
	Event      string    `json:"-"`
	ID         uuid.UUID `json:"id"`
	PracticeID uuid.UUID `json:"practice_id"`
}

type appointmentEventHub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uuid.UUID]map[uint64]chan appointmentInvalidation
}

func newAppointmentEventHub() *appointmentEventHub {
	return &appointmentEventHub{subscribers: make(map[uuid.UUID]map[uint64]chan appointmentInvalidation)}
}

func (h *appointmentEventHub) subscribe(practiceID uuid.UUID) (<-chan appointmentInvalidation, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	practiceSubscribers := h.subscribers[practiceID]
	if practiceSubscribers == nil {
		practiceSubscribers = make(map[uint64]chan appointmentInvalidation)
		h.subscribers[practiceID] = practiceSubscribers
	}
	if len(practiceSubscribers) >= appointmentSSEMaxSubscribers {
		return nil, nil, errors.New("appointment event subscriber limit reached")
	}
	h.nextID++
	subscriberID := h.nextID
	events := make(chan appointmentInvalidation, 1)
	practiceSubscribers[subscriberID] = events
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(practiceSubscribers, subscriberID)
			if len(practiceSubscribers) == 0 {
				delete(h.subscribers, practiceID)
			}
		})
	}
	return events, unsubscribe, nil
}

func (h *appointmentEventHub) BroadcastAppointmentChange(ctx context.Context, practiceID uuid.UUID, event string, appointmentID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event != "INSERT" && event != "UPDATE" && event != "DELETE" {
		return fmt.Errorf("invalid appointment broadcast event %q", event)
	}
	invalidation := appointmentInvalidation{Event: event, ID: appointmentID, PracticeID: practiceID}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, subscriber := range h.subscribers[practiceID] {
		select {
		case subscriber <- invalidation:
		default:
			// Every event causes a full refetch, so retaining only the latest event is safe.
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- invalidation:
			default:
			}
		}
	}
	return nil
}

func (h *appointmentEventHub) subscriberCount(practiceID uuid.UUID) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers[practiceID])
}

func (s *Server) handlerAppointmentEvents(c *gin.Context) {
	user := c.MustGet("user").(AuthUser)
	if user.PracticeId == nil {
		respondWithError(c, http.StatusForbidden, "Practice membership required", nil)
		return
	}
	if s.appointmentEventHub == nil {
		respondWithError(c, http.StatusServiceUnavailable, "Appointment events are unavailable", nil)
		return
	}
	events, unsubscribe, err := s.appointmentEventHub.subscribe(*user.PracticeId)
	if err != nil {
		respondWithError(c, http.StatusServiceUnavailable, "Appointment events are at capacity", nil)
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Connection", "keep-alive")
	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
	if _, err := c.Writer.WriteString(": connected\n\n"); err != nil {
		return
	}
	c.Writer.Flush()

	heartbeat := time.NewTicker(appointmentSSEHeartbeat)
	defer heartbeat.Stop()
	maxLifetime := time.NewTimer(appointmentSSEMaxLifetime)
	defer maxLifetime.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-maxLifetime.C:
			return
		case <-heartbeat.C:
			if _, err := c.Writer.WriteString(": heartbeat\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case event := <-events:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Event, payload); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

type appointmentEventFanout struct {
	calendar *googleCalendarService
	hub      *appointmentEventHub
}

func (f *appointmentEventFanout) BroadcastAppointmentChange(ctx context.Context, practiceID uuid.UUID, event string, appointmentID uuid.UUID) error {
	return f.hub.BroadcastAppointmentChange(ctx, practiceID, event, appointmentID)
}

func (f *appointmentEventFanout) SyncAppointmentCreated(ctx context.Context, event AppointmentEvent) error {
	return f.calendar.SyncAppointmentCreated(ctx, event)
}

func (f *appointmentEventFanout) SyncAppointmentUpdated(ctx context.Context, event AppointmentEvent) error {
	return f.calendar.SyncAppointmentUpdated(ctx, event)
}

func (f *appointmentEventFanout) SyncAppointmentCancelled(ctx context.Context, practiceID, appointmentID uuid.UUID) error {
	return f.calendar.SyncAppointmentCancelled(ctx, practiceID, appointmentID)
}

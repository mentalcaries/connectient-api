package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"github.com/mentalcaries/connectient-api/internal/database"
)

type Server struct {
	port                int
	db                  database.Service
	DBQuery             *database.Queries
	storage             ObjectStorage
	registrationNotify  RegistrationNotifier
	teamInviteNotify    TeamInviteNotifier
	identityProfiles    IdentityProfileService
	appointmentNotify   AppointmentNotifier
	appointmentEvents   AppointmentEventService
	publicBookingNotify PublicAppointmentRequestNotifier
	patientBaseURL      string
	inviteBaseURL       string
}

func NewServer() *http.Server {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		log.Fatalf("invalid PORT value %q: %v", os.Getenv("PORT"), err)
	}

	db := database.NewDb()
	queries := database.New(db.Pool())
	notifications := newOutboundNotificationProvider(queries)

	AppServer := &Server{
		port:                port,
		db:                  db,
		DBQuery:             queries,
		storage:             newR2ObjectStorageFromEnv(),
		registrationNotify:  notifications,
		teamInviteNotify:    notifications,
		appointmentNotify:   notifications,
		publicBookingNotify: notifications,
		patientBaseURL:      strings.TrimRight(os.Getenv("PATIENT_URL"), "/"),
		inviteBaseURL:       strings.TrimRight(os.Getenv("FRONTEND_BASE_URL"), "/"),
		identityProfiles:    newHTTPIdentityProfileServiceFromEnv(),
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", AppServer.port),
		Handler:      AppServer.RegisterRoutes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return server
}

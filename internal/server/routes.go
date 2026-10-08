package server

import (
	"errors"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		user, err := s.UserFromRequest(c)
		if err != nil {
			switch {
			case errors.Is(err, ErrMembershipDenied):
				respondWithError(c, http.StatusForbidden, "practice access denied", nil)
			case errors.Is(err, ErrMembershipLookup):
				respondWithError(c, http.StatusInternalServerError, "could not verify practice access", err)
			default:
				respondWithError(c, http.StatusUnauthorized, "authorization required", err)
			}
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func requireOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(AuthUser)
		if user.Role == nil || *user.Role != "owner" {
			respondWithError(c, http.StatusForbidden, "owner access required", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(AuthUser)
		if user.Role == nil || (*user.Role != "owner" && *user.Role != "admin") {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) ClaimsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		claims, err := s.ClaimsFromRequest(c)
		if err != nil {
			respondWithError(c, http.StatusUnauthorized, "authorization required", err)
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

func (s *Server) RegisterRoutes() http.Handler {
	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key"},
		AllowCredentials: true,
	}))

	public := router.Group("/")
	{
		public.GET("/", s.handleReadiness)
		public.GET("/health", s.healthHandler)
		public.GET("/public/practices/:code/booking-config", s.handlerGetPublicBookingConfig)
		public.GET("/public/practices/:code/procedure-types", s.handlerGetPublicProcedureTypes)
		public.GET("/public/practices/:code/locations", s.handlerGetPublicLocations)
		public.POST("/public/practices/:code/appointment-requests", s.handlerCreatePublicAppointmentRequest)
		public.GET("/registrations/form/:token", s.handlerGetPublicRegistrationForm)
		public.POST("/registrations/form/:token", s.handlerSubmitPublicRegistrationForm)
		public.GET("/invite/validate", s.handlerValidateInvite)
		public.GET("/connected-apps/google/callback", s.handlerGoogleCalendarCallback)
		public.POST("/internal/auth/membership", s.handlerInternalMembershipLookup)
		public.DELETE("/internal/test/memberships/:id", s.handlerDeleteTestMembership)

	}

	claimsOnly := router.Group("/")
	claimsOnly.Use(s.ClaimsMiddleware())
	{
		claimsOnly.GET("/onboarding/check-code", s.handlerCheckCodeAvailability)
		claimsOnly.GET("/onboarding/suggest-code", s.handlerSuggestPracticeCode)
		claimsOnly.POST("/onboarding/complete", s.handlerCompleteOnboarding)
		claimsOnly.GET("/onboarding/progress", s.handlerGetOnboardingProgress)
		claimsOnly.PATCH("/onboarding/progress/:key", s.handlerPatchOnboardingProgress)
		claimsOnly.GET("/users/me", s.handlerGetCurrentUser)
		claimsOnly.GET("/me/context", s.handlerGetCurrentUserContext)
		claimsOnly.POST("/invite/accept", s.handlerAcceptInvite)

	}

	authenticated := router.Group("/")
	authenticated.Use(s.AuthMiddleware())
	{
		authenticated.GET("/appointments", s.handlerGetAllAppointments)
		authenticated.GET("/appointments/events", s.handlerAppointmentEvents)
		authenticated.POST("/appointments", s.handlerCreateStaffAppointment)
		authenticated.GET("/appointments/availability", s.handlerGetAppointmentAvailability)
		authenticated.GET("/appointments/:id", s.handlerGetAppointmentById)
		authenticated.PATCH("/appointments/:id", s.handlerAppointmentsUpdate)
		authenticated.POST("/appointments/:id/read", s.handlerMarkAppointmentRead)
		authenticated.POST("/appointments/:id/cancel", s.handlerCancelAppointment)
		authenticated.GET("/appointments/confirmed", s.handlerGetConfirmedAppointments)
		authenticated.POST("/appointments/:id/schedule", s.handlerScheduleAppointment)
		authenticated.POST("/appointments/:id/confirm", s.handlerConfirmAppointment)
		authenticated.GET("/patients", s.handlerListPatients)
		authenticated.GET("/patients/:id", s.handlerGetPatient)
		authenticated.PATCH("/patients/:id", s.handlerPatchPatient)
		authenticated.GET("/patients/:id/appointments", s.handlerGetPatientAppointments)
		authenticated.GET("/patients/:id/registrations", s.handlerGetLatestPatientRegistration)
		authenticated.GET("/registrations", s.handlerListRegistrations)
		authenticated.POST("/registrations", s.handlerCreateRegistration)
		authenticated.GET("/registrations/:id", s.handlerGetRegistration)
		authenticated.DELETE("/registrations/:id", requireAdmin(), s.handlerDeleteRegistration)
		authenticated.GET("/registrations/:id/link", s.handlerGetRegistrationLink)
		authenticated.POST("/registrations/:id/send-email", s.handlerSendRegistrationEmail)
		authenticated.POST("/registrations/:id/send-whatsapp", s.handlerSendRegistrationWhatsApp)
		authenticated.POST("/registrations/:id/resend", s.handlerResendRegistration)
		authenticated.GET("/users", requireAdmin(), s.handlerListPracticeUsers)
		authenticated.PATCH("/users/:id", requireAdmin(), s.handlerPatchPracticeUser)
		authenticated.DELETE("/users/:id", requireAdmin(), s.handlerDeletePracticeUser)
		authenticated.GET("/users/seats", requireAdmin(), s.handlerGetPracticeSeats)
		authenticated.GET("/users/invites", requireAdmin(), s.handlerListPracticeInvites)
		authenticated.POST("/users/invites", requireAdmin(), s.handlerCreatePracticeInvite)
		authenticated.DELETE("/users/invites/:id", requireAdmin(), s.handlerDeletePracticeInvite)
		authenticated.POST("/users/invites/:id/resend", requireAdmin(), s.handlerResendPracticeInvite)

		authenticated.GET("/practices", s.handlerGetPracticeWithSettings)
		authenticated.GET("/practices/settings", s.handlerGetPracticeSettings)
		authenticated.PATCH("/practices/settings", requireAdmin(), s.handlerPatchPracticeSettings)
		authenticated.GET("/practices/locations", s.handlerGetPracticeLocations)
		authenticated.POST("/practices/locations", requireAdmin(), s.handlerCreatePracticeLocation)
		authenticated.PATCH("/practices/locations/:id", requireAdmin(), s.handlerPatchPracticeLocation)
		authenticated.GET("/practices/procedure-types", s.handlerGetPracticeProcedures)
		authenticated.POST("/practices/procedure-types", requireAdmin(), s.handlerCreateProcedureType)
		authenticated.PATCH("/practices/procedure-types/:id", requireAdmin(), s.handlerPatchProcedureType)
		authenticated.DELETE("/practices/procedure-types/:id", requireAdmin(), s.handlerDeleteProcedureType)
		authenticated.GET("/practices/providers", s.handlerGetPracticeProviders)
		authenticated.POST("/practices/providers", requireAdmin(), s.handlerCreatePracticeProvider)
		authenticated.PATCH("/practices/providers/:id", requireAdmin(), s.handlerPatchPracticeProvider)
		authenticated.PUT("/practices/providers/:id/main", requireAdmin(), s.handlerSetMainPracticeProvider)
		authenticated.DELETE("/practices/providers/:id", requireAdmin(), s.handlerDeletePracticeProvider)
		authenticated.GET("/practices/profile", s.handlerGetPracticeProfile)
		authenticated.GET("/practices/subscription", s.handlerGetPracticeSubscription)
		authenticated.PATCH("/practices/profile", requireAdmin(), s.handlerPatchPracticeProfile)
		authenticated.PATCH("/practices/practice-code", requireAdmin(), s.handlerPatchPracticeCode)
		authenticated.GET("/account", s.handlerGetAccount)
		authenticated.PATCH("/account", s.handlerPatchAccount)
		authenticated.POST("/upload/avatar", s.handlerUploadAvatar)
		authenticated.DELETE("/upload/avatar", s.handlerDeleteAvatar)
		authenticated.POST("/upload/logo", requireAdmin(), s.handlerUploadPracticeLogo)
		authenticated.DELETE("/upload/logo", requireAdmin(), s.handlerDeletePracticeLogo)
		authenticated.GET("/connected-apps", requireOwner(), s.handlerGetConnectedApps)
		authenticated.GET("/connected-apps/google/auth", requireOwner(), s.handlerGoogleCalendarAuth)
		authenticated.DELETE("/connected-apps/google", requireOwner(), s.handlerDeleteGoogleCalendar)
		authenticated.GET("/export/appointments", requireAdmin(), s.handlerExportAppointments)
		authenticated.GET("/export/registrations", requireAdmin(), s.handlerExportRegistrations)
	}

	return router
}

func (s *Server) handleReadiness(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{"message": "Connectient up"})
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}

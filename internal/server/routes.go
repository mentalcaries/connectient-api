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
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	public := router.Group("/")
	{
		public.GET("/", s.handleReadiness)
		public.GET("/health", s.healthHandler)
		public.POST("/appointments", s.handlerAppointmentsCreate)
		public.GET("/register/suggest-code", s.handlerSuggestPracticeCode)
		public.GET("/register/check-code", s.handlerCheckCodeAvailability)

	}

	claimsOnly := router.Group("/")
	claimsOnly.Use(s.ClaimsMiddleware())
	{
		claimsOnly.POST("/register", s.handlerNewRegistration)
		claimsOnly.GET("/users/me", s.handlerGetCurrentUser)
		claimsOnly.GET("/me/context", s.handlerGetCurrentUserContext)

	}

	authenticated := router.Group("/")
	authenticated.Use(s.AuthMiddleware())
	{
		authenticated.GET("/appointments", s.handlerGetAllAppointments)
		authenticated.GET("appointments/:id", s.handlerGetAppointmentById)
		authenticated.PATCH("/appointments/:id", s.handlerAppointmentsUpdate)
		authenticated.DELETE("/appointments/:id", s.handlerAppointmentsDelete)
		authenticated.GET("/appointments/confirmed", s.handlerGetConfirmedAppointments)

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
		authenticated.GET("/practices/connected-apps", requireOwner(), s.handlerGetConnectedApps)
	}

	return router
}

func (s *Server) handleReadiness(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{"message": "Connectient up"})
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}

package server

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		user, err := s.UserFromRequest(c)
		if err != nil {
			respondWithError(c, http.StatusUnauthorized, "authorization required", err)
			c.Abort()
			return
		}
		c.Set("user", user)
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

	}

	authenticated := router.Group("/")
	authenticated.Use(s.AuthMiddleware())
	{
		authenticated.GET("/appointments", s.handlerAppointmentsGetAll)
		authenticated.GET("appointments/:id", s.handlerGetAppointmentById)
		authenticated.PATCH("/appointments", s.handlerAppointmentsUpdate)
		authenticated.DELETE("/appointments/:id", s.handlerAppointmentsDelete)

	}

	return router
}

func (s *Server) handleReadiness(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{"message": "Connectient up"})
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}

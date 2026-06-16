package server

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func (s *Server) RegisterRoutes() http.Handler {
	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3000"}, // Add your frontend URL
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true, // Enable cookies/auth
	}))

	router.GET("/", s.handleReadiness)
	router.GET("/health", s.healthHandler)

	router.POST("/users", s.handlerUserCreate)

	router.GET("/appointments", s.handlerAppointmentsGetAll)
	router.GET("appointments/:id", s.handlerGetAppointmentById)
	router.POST("/appointments", s.handlerAppointmentsCreate)
	router.PATCH("/appointments", s.handlerAppointmentsUpdate)
	router.DELETE("/appointments/:id", s.handlerAppointmentsDelete)

	router.GET("/register/suggest-code", s.handlerSuggestPracticeCode)
	router.GET("/register/check-code", s.handlerCheckCodeAvailability)
	

	return router
}

func (s *Server) handleReadiness(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{"message": "Connectient up"})
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.db.Health())
}

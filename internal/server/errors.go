package server

import (
	"log"

	"github.com/gin-gonic/gin"
)


func respondWithError(c *gin.Context, code int, message string, err error) {
	if err != nil {
		log.Println(err)
	}

	if code > 499 {
		log.Printf("Responding with a %v error: %s", code, message)
	}
	c.JSON(code, gin.H{"error": message})

}

package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/joho/godotenv/autoload"
)

type HealthStatus struct {
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
	Error         string `json:"error,omitempty"`
	TotalConns    int32  `json:"total_conns"`
	AcquiredConns int32  `json:"acquired_conns"`
	IdleConns     int32  `json:"idle_conns"`
	MaxConns      int32  `json:"max_conns"`
}

type Service interface {
	Health() HealthStatus
	Pool() *pgxpool.Pool
	Close() error
}

type service struct {
	pool *pgxpool.Pool
}

var dbInstance *service

func NewDb() Service {
	if dbInstance != nil {
		return dbInstance
	}
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatal(err)
	}
	dbInstance = &service{pool: pool}
	return dbInstance
}

func (s *service) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *service) Health() HealthStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := s.pool.Ping(ctx); err != nil {
		return HealthStatus{
			Status: "down",
			Error:  fmt.Sprintf("db down: %v", err),
		}
	}

	stats := s.pool.Stat()
	status := HealthStatus{
		Status:        "up",
		Message:       "It's healthy",
		TotalConns:    stats.TotalConns(),
		AcquiredConns: stats.AcquiredConns(),
		IdleConns:     stats.IdleConns(),
		MaxConns:      stats.MaxConns(),
	}

	if stats.AcquiredConns() > int32(float32(stats.MaxConns())*0.8) {
		status.Message = "The database is experiencing heavy load."
	}

	return status
}

func (s *service) Close() error {
	log.Println("Disconnected from database")
	s.pool.Close()
	return nil
}

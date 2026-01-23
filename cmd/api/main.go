package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/esports-scouting-backend/internal/config"
	"github.com/yourusername/esports-scouting-backend/internal/grid"
	"github.com/yourusername/esports-scouting-backend/internal/handlers"
	"github.com/yourusername/esports-scouting-backend/internal/repository"
	"github.com/yourusername/esports-scouting-backend/pkg/cache"
)

func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Connect to Postgres (NeonDB)
	pgRepo, err := repository.NewPostgresRepo(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	if err := pgRepo.RunMigrations(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	// 3. Connect to Redis (Redis Cloud)
	redisCache, err := cache.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// 4. Initialize Grid API Client
	gridClient := grid.NewClient(cfg.GridAPIKey)

	// 5. Setup Gin
	router := gin.Default()

	// CORS Setup
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin == "http://localhost:5173" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Trusted Proxies
	if cfg.TrustedProxies != "" {
		proxies := strings.Split(cfg.TrustedProxies, ",")
		if err := router.SetTrustedProxies(proxies); err != nil {
			log.Printf("Warning: Failed to set trusted proxies: %v", err)
		}
	}

	// Handlers
	h := handlers.NewHandler(pgRepo, redisCache, gridClient)

	// Health Check
	router.GET("/health", h.HealthCheck)

	// API v1 Routes
	v1 := router.Group("/api/v1")
	{
		// Core comparison & trends
		v1.GET("/compare", h.CompareTeams)
		v1.GET("/team/trends", h.GetTeamTrends)

		// Discovery endpoints
		v1.GET("/titles", h.GetAvailableTitles)
		v1.GET("/tournaments", h.GetAvailableTournaments)
		v1.GET("/teams", h.GetAvailableTeams)

		// ✅ FEATURE #7: Meta Analysis & Scouting Report
		//v1.GET("/meta", h.GetMeta)
		v1.GET("/scouting-report", h.GenerateScoutingReport)
		v1.GET("/teams/search", h.SearchTeams)
	}

	// 6. Start server with Graceful Shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		fmt.Printf("🚀 Server starting on port %s\n", cfg.Port)
		fmt.Println("📊 Available endpoints:")
		fmt.Println("   GET /health")
		fmt.Println("   GET /api/v1/compare")
		fmt.Println("   GET /api/v1/team/trends")
		fmt.Println("   GET /api/v1/scouting-report")
		fmt.Println("   GET /api/v1/meta")
		fmt.Println("   GET /api/v1/teams/search")
		fmt.Println("   GET /api/v1/teams")
		fmt.Println("   GET /api/v1/titles")
		fmt.Println("   GET /api/v1/tournaments")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("✅ Server exited")
}
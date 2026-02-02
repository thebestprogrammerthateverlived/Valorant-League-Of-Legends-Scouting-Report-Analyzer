package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "sync"
    "syscall"
    "time"

    "github.com/gin-gonic/gin"
    "golang.org/x/time/rate"
    "github.com/yourusername/esports-scouting-backend/internal/config"
    "github.com/yourusername/esports-scouting-backend/internal/grid"
    "github.com/yourusername/esports-scouting-backend/internal/handlers"
    "github.com/yourusername/esports-scouting-backend/internal/repository"
    "github.com/yourusername/esports-scouting-backend/pkg/cache"
)

// ============================================================================
// RATE LIMITER
// ============================================================================
type IPRateLimiter struct {
    ips map[string]*rate.Limiter
    mu  *sync.RWMutex
    r   rate.Limit
    b   int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
    return &IPRateLimiter{
        ips: make(map[string]*rate.Limiter),
        mu:  &sync.RWMutex{},
        r:   r,
        b:   b,
    }
}

func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
    i.mu.Lock()
    defer i.mu.Unlock()

    limiter, exists := i.ips[ip]
    if !exists {
        limiter = rate.NewLimiter(i.r, i.b)
        i.ips[ip] = limiter
    }
    return limiter
}

func rateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
    return func(c *gin.Context) {
        ip := c.ClientIP()
        l := limiter.GetLimiter(ip)

        if !l.Allow() {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded. Please try again later.",
                "retry_after": "60s",
            })
            c.Abort()
            return
        }
        c.Next()
    }
}

// ============================================================================
// SECURITY HEADERS
// ============================================================================
func securityHeadersMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
        c.Next()
    }
}

// ============================================================================
// CORS MIDDLEWARE
// ============================================================================
func corsMiddleware() gin.HandlerFunc {
    allowedOrigins := map[string]bool{
        "https://frontend-esports-analyzer-valorant.vercel.app":            true,
    }

    return func(c *gin.Context) {
        origin := c.Request.Header.Get("Origin")

        if allowedOrigins[origin] {
            c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
            c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
            c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
            c.Writer.Header().Set("Access-Control-Max-Age", "3600")
        }

        if c.Request.Method == "OPTIONS" {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }

        c.Next()
    }
}

func main() {
    // 1. Load config
    cfg, err := config.Load()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // 2. Connect to Postgres
    pgRepo, err := repository.NewPostgresRepo(cfg.DatabaseURL)
    if err != nil {
        log.Fatalf("Failed to connect to Postgres: %v", err)
    }
    if err := pgRepo.RunMigrations(); err != nil {
        log.Fatalf("Failed to create tables: %v", err)
    }

    // 3. Connect to Redis
    redisCache, err := cache.NewRedisClient(cfg.RedisURL)
    if err != nil {
        log.Fatalf("Failed to connect to Redis: %v", err)
    }

    // 4. Initialize Grid API Client
    gridClient := grid.NewClient(cfg.GridAPIKey)

    // 5. Setup Gin
    router := gin.Default()
    
    // Apply middleware
    router.Use(corsMiddleware())
    router.Use(securityHeadersMiddleware())
    
    limiter := NewIPRateLimiter(10, 20)
    router.Use(rateLimitMiddleware(limiter))

    // 6. Initialize handlers
    handler := handlers.NewHandler(pgRepo, redisCache, gridClient)

    // 7. Routes
    router.GET("/health", handler.HealthCheck)

    // API routes
    api := router.Group("/api/v1")
    {
        // Comparison & Analysis
        api.GET("/compare", handler.CompareTeams)
        api.GET("/trends", handler.GetTeamTrends)
        api.GET("/meta", handler.GetMeta)
        
        // Scouting Report (comprehensive)
        api.GET("/scouting-report", handler.GenerateScoutingReport)
        
        // Search & Discovery
        api.GET("/search", handler.SearchTeams)        
        api.GET("/teams/search", handler.SearchTeams)
        api.GET("/teams", handler.GetAvailableTeams)
        api.GET("/titles", handler.GetAvailableTitles)
        api.GET("/tournaments", handler.GetAvailableTournaments)
    }

    // 8. Start server with graceful shutdown
    srv := &http.Server{
        Addr:    ":8080",
        Handler: router,
    }

    go func() {
        log.Println("🚀 Server starting on :8080")
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Server failed: %v", err)
        }
    }()

    // Wait for interrupt signal
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Println("Shutting down server...")
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := srv.Shutdown(ctx); err != nil {
        log.Fatal("Server forced to shutdown:", err)
    }

    log.Println("Server stopped")
}
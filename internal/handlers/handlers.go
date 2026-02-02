package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/esports-scouting-backend/internal/grid"
	"github.com/yourusername/esports-scouting-backend/internal/models"
	"github.com/yourusername/esports-scouting-backend/internal/repository"
	"github.com/yourusername/esports-scouting-backend/internal/services"
	"github.com/yourusername/esports-scouting-backend/pkg/cache"
)

type Handler struct {
	pgRepo        *repository.PostgresRepo
	redisCache    *cache.RedisClient
	gridClient    *grid.Client
	compService   *services.ComparisonService
	trendsService *services.TrendsService
	metaService   *services.MetaService      // ✅ NEW
	reportService *services.ReportService    // ✅ NEW
}

func NewHandler(pg *repository.PostgresRepo, redis *cache.RedisClient, grid *grid.Client) *Handler {
	return &Handler{
		pgRepo:        pg,
		redisCache:    redis,
		gridClient:    grid,
		compService:   services.NewComparisonService(grid, redis, pg),
		trendsService: services.NewTrendsService(grid, redis),
		metaService:   services.NewMetaService(grid, redis),        //  NEW
		reportService: services.NewReportService(grid, redis, pg),  //  NEW
	}
}

func (h *Handler) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	postgresStatus := h.pgRepo.HealthCheck()
	redisStatus := h.redisCache.HealthCheck(ctx)
	gridStatus := h.gridClient.HealthCheck(ctx)

	status := "ok"
	if !postgresStatus || !redisStatus || !gridStatus {
		status = "error"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"postgres":  postgresStatus,
		"redis":     redisStatus,
		"grid_api":  gridStatus,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (h *Handler) CompareTeams(c *gin.Context) {
	start := time.Now()
	team1 := c.Query("team1")
	team2 := c.Query("team2")
	title := c.Query("title")
	timeWindow := models.TimeWindow(c.Query("timeWindow"))
	tournamentIDsParam := c.Query("tournamentIds")

	if team1 == "" || team2 == "" || title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "team1, team2, and title are required",
			"example": "/api/v1/compare?team1=Cloud9&team2=Sentinels&title=valorant",
		})
		return
	}

	// ✅ Validate title parameter
	title = strings.ToLower(title)
	if title != "valorant" && title != "lol" && title != "leagueoflegends" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid title parameter",
			"message": "title must be 'valorant' or 'lol'",
			"provided": title,
		})
		return
	}

	if timeWindow == "" {
		timeWindow = models.Last3Months
	}

	var tournamentIDs []string
	if tournamentIDsParam != "" {
		tournamentIDs = strings.Split(tournamentIDsParam, ",")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("compare:%s:%s:%s:%s:%s", team1, team2, title, timeWindow, tournamentIDsParam)
	var cachedReport models.ComparisonReport
	err := h.redisCache.Get(ctx, cacheKey, &cachedReport)
	if err == nil {
		log.Printf("[CACHE HIT] CompareTeams took %v", time.Since(start))
		c.JSON(http.StatusOK, cachedReport)
		return
	}

	report, err := h.compService.CompareTeams(ctx, team1, team2, title, timeWindow, tournamentIDs)
	if err != nil {
		log.Printf("[ERROR] Comparison failed: %v", err)

		// ✅ Check for InsufficientDataError (404)
		var dataErr *grid.InsufficientDataError
		if errors.As(err, &dataErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   dataErr.Error(),
				"team":    dataErr.TeamName,
				"reason":  dataErr.Reason,
				"title":   title,
				"message": fmt.Sprintf("Team '%s' has insufficient data in %s. Check team name and title parameter.", dataErr.TeamName, title),
			})
			return
		}

		var teamErr *grid.TeamNotFoundError
		if errors.As(err, &teamErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":          teamErr.Error(),
				"team":           teamErr.TeamName,
				"title":          title,
				"availableTeams": teamErr.AvailableTeams,
				"message":        fmt.Sprintf("Team '%s' not found in %s. Check the team name and title parameter.", teamErr.TeamName, title),
			})
			return
		}

		if strings.Contains(err.Error(), "no teams found matching name") {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   err.Error(),
				"title":   title,
				"message": fmt.Sprintf("Team not found in %s. Verify both team names exist in this game title.", title),
			})
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error":   "Request timeout",
				"message": "The request took too long to complete. Try again or use a shorter time window.",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.redisCache.Set(ctx, cacheKey, report, 1*time.Hour); err != nil {
		log.Printf("Warning: Failed to cache comparison: %v", err)
	}

	log.Printf("[CACHE MISS] CompareTeams took %v", time.Since(start))
	c.JSON(http.StatusOK, report)
}

func (h *Handler) GetTeamTrends(c *gin.Context) {
	start := time.Now()
	teamName := c.Query("name")
	title := c.Query("title")
	tournamentIDsParam := c.Query("tournamentIds")

	if teamName == "" || title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and title are required"})
		return
	}

	var tournamentIDs []string
	if tournamentIDsParam != "" {
		tournamentIDs = strings.Split(tournamentIDsParam, ",")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	cacheKey := fmt.Sprintf("trends:%s:%s:%s", teamName, title, tournamentIDsParam)
	var cachedTrends models.TrendReport
	err := h.redisCache.Get(ctx, cacheKey, &cachedTrends)
	if err == nil {
		log.Printf("[CACHE HIT] GetTeamTrends took %v", time.Since(start))
		c.JSON(http.StatusOK, cachedTrends)
		return
	}

	trends, err := h.trendsService.AnalyzeTrends(ctx, teamName, title, tournamentIDs)
	if err != nil {
		log.Printf("[ERROR] Trends analysis failed: %v", err)

		// ✅ Check for InsufficientDataError (404 - team exists but no data)
		var dataErr *grid.InsufficientDataError
		if errors.As(err, &dataErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   dataErr.Error(),
				"team":    dataErr.TeamName,
				"reason":  dataErr.Reason,
				"message": "Team has insufficient data available. Try a team with recent matches.",
			})
			return
		}

		// Check for TeamNotFoundError (404 - team doesn't exist)
		var teamErr *grid.TeamNotFoundError
		if errors.As(err, &teamErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":          teamErr.Error(),
				"team":           teamErr.TeamName,
				"availableTeams": teamErr.AvailableTeams,
				"message":        fmt.Sprintf("Team '%s' did not play in the available tournaments. Check the title val or lol and try again", teamErr.TeamName,),
			})
			return
		}

		if strings.Contains(err.Error(), "no teams found matching name") {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   err.Error(),
				"message": "Team not found in this game title. Check the team name and title parameter.",
			})
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error":   "Request timeout",
				"message": "The analysis took too long to complete. Try again later.",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.redisCache.Set(ctx, cacheKey, trends, 3*time.Hour); err != nil {
		log.Printf("Warning: Failed to cache trends: %v", err)
	}

	log.Printf("[CACHE MISS] GetTeamTrends took %v", time.Since(start))
	c.JSON(http.StatusOK, trends)
}

// ============================================================================
// FEATURE #7: NEW ENDPOINTS
// ============================================================================

// GetMeta returns meta analysis for a game title
func (h *Handler) GetMeta(c *gin.Context) {
	title := c.Query("title")
	tournamentID := c.Query("tournamentId")

	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "title parameter is required",
			"example": "/api/v1/meta?title=valorant",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	report, err := h.metaService.AnalyzeMeta(ctx, title, tournamentID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   err.Error(),
			"message": "Meta analysis requires Grid.gg pick/ban data API (not available in current tier)",
			"note":    "Use team statistics endpoints for performance analysis",
		})
		return
	}

	c.JSON(http.StatusOK, report)
}

// GenerateScoutingReport creates comprehensive scouting report
func (h *Handler) GenerateScoutingReport(c *gin.Context) {
	start := time.Now()
	opponent := c.Query("opponent")
	myTeam := c.Query("myTeam")
	title := c.Query("title")
	timeWindow := models.TimeWindow(c.Query("timeWindow"))
	tournamentIDsParam := c.Query("tournamentIds")

	if opponent == "" || myTeam == "" || title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "opponent, myTeam, and title are required",
			"example": "/api/v1/scouting-report?opponent=G2%20Esports&myTeam=Cloud9&title=valorant",
		})
		return
	}

	if timeWindow == "" {
		timeWindow = models.Last3Months
	}

	var tournamentIDs []string
	if tournamentIDsParam != "" {
		tournamentIDs = strings.Split(tournamentIDsParam, ",")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	report, err := h.reportService.GenerateScoutingReport(ctx, opponent, myTeam, title, timeWindow, tournamentIDs)
	if err != nil {
		log.Printf("[ERROR] Scouting report generation failed: %v", err)

		var teamErr *grid.TeamNotFoundError
		if errors.As(err, &teamErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":          teamErr.Error(),
				"team":           teamErr.TeamName,
				"availableTeams": teamErr.AvailableTeams,
			})
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			c.JSON(http.StatusGatewayTimeout, gin.H{
				"error":   "Request timeout",
				"message": "Report generation took too long. Try using cached data or a shorter time window.",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SUCCESS] Generated scouting report in %v (cached: %v)", time.Since(start), report.CacheStatus.FromCache)
	c.JSON(http.StatusOK, report)
}

// SearchTeams provides autocomplete for team names
// func (h *Handler) SearchTeams(c *gin.Context) {
// 	query := strings.ToLower(c.Query("q"))
// 	title := c.Query("title")

// 	if query == "" || title == "" {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"error":   "q and title parameters are required",
// 			"example": "/api/v1/teams/search?q=cloud&title=valorant",
// 		})
// 		return
// 	}

// 	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
// 	defer cancel()

// 	// Fetch all teams and filter
// 	teams, err := h.gridClient.GetAvailableTeams(ctx, title, nil)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}

// 	// Filter and rank results
// 	var results []models.TeamSearchResult
// 	for _, teamName := range teams {
// 		lowerName := strings.ToLower(teamName)
// 		if strings.Contains(lowerName, query) {
// 			relevance := 50
// 			if strings.HasPrefix(lowerName, query) {
// 				relevance = 100
// 			} else if strings.HasPrefix(lowerName, query[:1]) {
// 				relevance = 75
// 			}

// 			results = append(results, models.TeamSearchResult{
// 				Name:        teamName,
// 				DisplayName: teamName,
// 				Title:       title,
// 				Relevance:   relevance,
// 			})
// 		}

// 		if len(results) >= 10 {
// 			break
// 		}
// 	}

// 	c.JSON(http.StatusOK, gin.H{
// 		"query":   query,
// 		"results": results,
// 		"count":   len(results),
// 	})
// }

// SearchTeams provides autocomplete for team names
func (h *Handler) SearchTeams(c *gin.Context) {
	// ✅ FIXED: Accept both "query" and "q" parameters
	query := c.Query("query")
	if query == "" {
		query = c.Query("q") // Fallback to "q" for backwards compatibility
	}
	query = strings.ToLower(query)

	// ✅ FIXED: Accept both "game" and "title" parameters
	title := c.Query("game")
	if title == "" {
		title = c.Query("title") // Fallback to "title"
	}

	if query == "" || title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "query and game parameters are required",
			"example": "/api/v1/search?query=cloud&game=valorant",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Fetch all teams and filter
	teams, err := h.gridClient.GetAvailableTeams(ctx, title, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Filter and rank results
	var results []models.TeamSearchResult
	for _, teamName := range teams {
		lowerName := strings.ToLower(teamName)
		if strings.Contains(lowerName, query) {
			relevance := 50
			if strings.HasPrefix(lowerName, query) {
				relevance = 100
			} else if strings.HasPrefix(lowerName, query[:1]) {
				relevance = 75
			}

			results = append(results, models.TeamSearchResult{
				Name:        teamName,
				DisplayName: teamName,
				Title:       title,
				Relevance:   relevance,
			})
		}

		if len(results) >= 10 {
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"results": results,
		"count":   len(results),
	})
}

// ============================================================================
// EXISTING ENDPOINTS
// ============================================================================

func (h *Handler) GetAvailableTitles(c *gin.Context) {
	titles := []gin.H{
		{
			"id":          "6",
			"name":        "Valorant",
			"slug":        "valorant",
			"description": "Tactical FPS by Riot Games",
		},
		{
			"id":          "3",
			"name":        "League of Legends",
			"slug":        "lol",
			"aliases":     []string{"lol", "leagueoflegends"},
			"description": "MOBA by Riot Games",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"titles": titles,
		"count":  len(titles),
		"note":   "Use the 'slug' field as the 'title' parameter in other endpoints",
	})
}

func (h *Handler) GetAvailableTournaments(c *gin.Context) {
	titleFilter := c.Query("title")

	allTournaments := map[string][]gin.H{
		"valorant": {
			{"id": "757371", "name": "VCT Americas - Kickoff 2024"},
			{"id": "757481", "name": "VCT Americas - Stage 1 2024"},
			{"id": "774782", "name": "VCT Americas - Stage 2 2024"},
			{"id": "775516", "name": "VCT Americas - Kickoff 2025"},
			{"id": "800675", "name": "VCT Americas - Stage 1 2025"},
			{"id": "826660", "name": "VCT Americas - Stage 2 2025"},
		},
		"lol": {
			{"id": "758024", "name": "LCK - Spring 2024"},
			{"id": "774794", "name": "LCK - Summer 2024"},
			{"id": "825490", "name": "LCK - Split 2 2025"},
			{"id": "826679", "name": "LCK - Split 3 2025"},
			{"id": "758043", "name": "LCS - Spring 2024"},
			{"id": "774888", "name": "LCS - Summer 2024"},
			{"id": "758077", "name": "LEC - Spring 2024"},
			{"id": "774622", "name": "LEC - Summer 2024"},
			{"id": "825468", "name": "LEC - Spring 2025"},
			{"id": "826906", "name": "LEC - Summer 2025"},
			{"id": "758054", "name": "LPL - Spring 2024"},
			{"id": "774845", "name": "LPL - Summer 2024"},
			{"id": "775662", "name": "LPL - Split 1 2025"},
			{"id": "825450", "name": "LPL - Split 2 2025"},
		},
	}

	if titleFilter != "" {
		titleFilter = strings.ToLower(titleFilter)
		if tournaments, exists := allTournaments[titleFilter]; exists {
			c.JSON(http.StatusOK, gin.H{
				"title":       titleFilter,
				"tournaments": tournaments,
				"count":       len(tournaments),
				"note":        "Use the 'id' field as part of 'tournamentIds' parameter (comma-separated)",
			})
			return
		}

		c.JSON(http.StatusNotFound, gin.H{
			"error":           "Title not found",
			"availableTitles": []string{"valorant", "lol"},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tournaments": allTournaments,
		"note":        "Filter by title using ?title=valorant or ?title=lol",
	})
}

func (h *Handler) GetAvailableTeams(c *gin.Context) {
	titleParam := c.Query("title")
	tournamentIDsParam := c.Query("tournamentIds")
	validateData := c.Query("validateData") // ✅ NEW: Optional param to validate data access

	if titleParam == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "title parameter is required",
			"example": "/api/v1/teams?title=valorant",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second) // ✅ Increased timeout for validation
	defer cancel()

	var tournamentIDs []string
	if tournamentIDsParam != "" {
		tournamentIDs = strings.Split(tournamentIDsParam, ",")
	}

	// ✅ Use different cache key for validated teams
	cacheKey := fmt.Sprintf("teams:list:%s:%s:validated:%s", titleParam, tournamentIDsParam, validateData)
	var cachedTeams []string
	err := h.redisCache.Get(ctx, cacheKey, &cachedTeams)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"title":  titleParam,
			"teams":  cachedTeams,
			"count":  len(cachedTeams),
			"cached": true,
			"note":   "Only teams with accessible data are shown",
		})
		return
	}

	// ✅ Use validated endpoint by default (or when explicitly requested)
	var teams []string
	if validateData == "false" {
		// Legacy behavior - show all teams
		teams, err = h.gridClient.GetAvailableTeams(ctx, titleParam, tournamentIDs)
	} else {
		// ✅ NEW: Default to showing only teams with data access
		teams, err = h.gridClient.GetAvailableTeamsWithData(ctx, titleParam, tournamentIDs)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := h.redisCache.Set(ctx, cacheKey, teams, 6*time.Hour); err != nil {
		log.Printf("Warning: Failed to cache teams list: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"title": titleParam,
		"teams": teams,
		"count": len(teams),
		"note":  "Only teams with accessible Series State data. Use these names in other endpoints.",
	})
}
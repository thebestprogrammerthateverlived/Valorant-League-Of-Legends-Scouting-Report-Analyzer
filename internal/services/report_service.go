package services

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/esports-scouting-backend/internal/grid"
	"github.com/yourusername/esports-scouting-backend/internal/models"
	"github.com/yourusername/esports-scouting-backend/internal/repository"
	"github.com/yourusername/esports-scouting-backend/pkg/cache"
)

type ReportService struct {
	gridClient    *grid.Client
	cache         *cache.RedisClient
	pgRepo        *repository.PostgresRepo
	compService   *ComparisonService
	trendsService *TrendsService
	metaService   *MetaService
}

func NewReportService(gc *grid.Client, rc *cache.RedisClient, pg *repository.PostgresRepo) *ReportService {
	return &ReportService{
		gridClient:    gc,
		cache:         rc,
		pgRepo:        pg,
		compService:   NewComparisonService(gc, rc, pg),
		trendsService: NewTrendsService(gc, rc),
		metaService:   NewMetaService(gc, rc),
	}
}

// GenerateScoutingReport creates a comprehensive scouting report
func (s *ReportService) GenerateScoutingReport(
	ctx context.Context,
	opponent, myTeam, title string,
	timeWindow models.TimeWindow,
	tournamentIDs []string,
) (*models.ScoutingReport, error) {
	start := time.Now()
	cacheHit := false

	// Check cache first
	cacheKey := fmt.Sprintf("scouting:%s:%s:%s:%s", opponent, myTeam, title, timeWindow)
	var cachedReport models.ScoutingReport
	if err := s.cache.Get(ctx, cacheKey, &cachedReport); err == nil {
		cachedReport.CacheStatus = models.CacheStatus{
			FromCache: true,
			Age:       time.Since(cachedReport.GeneratedAt).String(),
		}
		return &cachedReport, nil
	}

	// Fetch all data in parallel for performance
	var (
		comparison *models.ComparisonReport
		trends1    *models.TrendReport
		trends2    *models.TrendReport
		metaCtx    *models.MetaContext
		wg         sync.WaitGroup
		mu         sync.Mutex
		errors     []error
	)

	// 1. Fetch comparison (required)
	wg.Add(1)
	go func() {
		defer wg.Done()
		comp, err := s.compService.CompareTeams(ctx, myTeam, opponent, title, timeWindow, tournamentIDs)
		mu.Lock()
		if err != nil {
			errors = append(errors, fmt.Errorf("comparison failed: %w", err))
		} else {
			comparison = comp
		}
		mu.Unlock()
	}()

	// 2. Fetch trends for your team
	wg.Add(1)
	go func() {
		defer wg.Done()
		t, err := s.trendsService.AnalyzeTrends(ctx, myTeam, title, tournamentIDs)
		mu.Lock()
		if err != nil {
			errors = append(errors, fmt.Errorf("trends for %s failed: %w", myTeam, err))
		} else {
			trends1 = t
		}
		mu.Unlock()
	}()

	// 3. Fetch trends for opponent
	wg.Add(1)
	go func() {
		defer wg.Done()
		t, err := s.trendsService.AnalyzeTrends(ctx, opponent, title, tournamentIDs)
		mu.Lock()
		if err != nil {
			errors = append(errors, fmt.Errorf("trends for %s failed: %w", opponent, err))
		} else {
			trends2 = t
		}
		mu.Unlock()
	}()

	// 4. Fetch meta context (optional, may fail gracefully)
	wg.Add(1)
	go func() {
		defer wg.Done()
		meta, _ := s.metaService.CompareTeamsToMeta(ctx, opponent, myTeam, title)
		mu.Lock()
		metaCtx = meta
		mu.Unlock()
	}()

	wg.Wait()

	// If comparison failed, we can't generate report
	if comparison == nil {
		if len(errors) > 0 {
			return nil, errors[0]
		}
		return nil, fmt.Errorf("failed to fetch comparison data")
	}

	// Build the report
	report := &models.ScoutingReport{
		ReportID:    uuid.New().String(),
		GeneratedAt: time.Now(),
		Matchup: models.MatchupInfo{
			Opponent: opponent,
			YourTeam: myTeam,
			Title:    title,
		},
		Comparison: *comparison,
		Trends: models.TrendsInfo{
			Opponent: models.TrendReport{},
			YourTeam: models.TrendReport{},
		},
		KeyInsights: []models.KeyInsight{},
		Confidence:  s.calculateOverallConfidence(comparison),
		CacheStatus: models.CacheStatus{
			FromCache: cacheHit,
			Age:       time.Since(start).String(),
		},
	}

	// Add trends if available
	if trends1 != nil {
		report.Trends.YourTeam = *trends1
	}
	if trends2 != nil {
		report.Trends.Opponent = *trends2
	}

	// Add meta context if available
	if metaCtx != nil {
		report.MetaContext = *metaCtx
	}

	// Generate key insights
	report.KeyInsights = s.generateKeyInsights(comparison, trends1, trends2)

	// Cache the report for 1 hour
	if err := s.cache.Set(ctx, cacheKey, report, 1*time.Hour); err != nil {
		// Log but don't fail
		fmt.Printf("[WARN] Failed to cache scouting report: %v\n", err)
	}

	fmt.Printf("[INFO] Generated scouting report in %v (cache: %v)\n", time.Since(start), cacheHit)

	return report, nil
}

// calculateOverallConfidence determines overall report confidence
func (s *ReportService) calculateOverallConfidence(comp *models.ComparisonReport) models.Confidence {
	team1Conf := comp.Team1.Stats.Confidence
	team2Conf := comp.Team2.Stats.Confidence

	// Use the lower confidence level
	lowestLevel := team1Conf.Level
	if team2Conf.Level == models.ConfidenceLow {
		lowestLevel = models.ConfidenceLow
	} else if team2Conf.Level == models.ConfidenceMedium && team1Conf.Level == models.ConfidenceHigh {
		lowestLevel = models.ConfidenceMedium
	}

	totalMatches := comp.DataQuality.Team1Matches + comp.DataQuality.Team2Matches
	avgMatches := totalMatches / 2

	return models.Confidence{
		Level:            lowestLevel,
		SampleSize:       avgMatches,
		Reasoning:        fmt.Sprintf("Based on %d matches analyzed across both teams", totalMatches),
		ReliabilityScore: (team1Conf.ReliabilityScore + team2Conf.ReliabilityScore) / 2,
	}
}

// generateKeyInsights creates prioritized insights from all data
func (s *ReportService) generateKeyInsights(
	comp *models.ComparisonReport,
	yourTrends *models.TrendReport,
	opponentTrends *models.TrendReport,
) []models.KeyInsight {
	var insights []models.KeyInsight

	// Check opponent's recent performance shifts (HIGH priority)
	if opponentTrends != nil {
		for _, alert := range opponentTrends.Alerts {
			if alert.Type == models.AlertPositiveShift && alert.Severity == models.AlertHigh {
				insights = append(insights, models.KeyInsight{
					Priority: "HIGH",
					Icon:     "🔴",
					Message:  fmt.Sprintf("Opponent %s - prepare for strong performance", alert.Message),
				})
			}
			if alert.Type == models.AlertNegativeShift && alert.Severity == models.AlertHigh {
				insights = append(insights, models.KeyInsight{
					Priority: "HIGH",
					Icon:     "🟢",
					Message:  fmt.Sprintf("Opponent struggling: %s - exploit this weakness", alert.Message),
				})
			}
		}
	}

	// Check your team's recent performance
	if yourTrends != nil {
		for _, alert := range yourTrends.Alerts {
			if alert.Type == models.AlertNegativeShift && alert.Severity == models.AlertHigh {
				insights = append(insights, models.KeyInsight{
					Priority: "HIGH",
					Icon:     "🔴",
					Message:  fmt.Sprintf("Your team: %s - needs adjustment", alert.Message),
				})
			}
		}
	}

	// Statistical advantages (MEDIUM priority)
	wrDiff := comp.Team1.Stats.WinRate - comp.Team2.Stats.WinRate
	if math.Abs(wrDiff) >= 0.15 {
		priority := "MEDIUM"
		icon := "🟡"
		var message string
		if wrDiff > 0 {
			message = fmt.Sprintf("You have significant win rate advantage (+%.0f%%)", wrDiff*100)
			icon = "🟢"
		} else {
			message = fmt.Sprintf("Opponent has win rate advantage (+%.0f%%)", -wrDiff*100)
			priority = "HIGH"
			icon = "🔴"
		}
		insights = append(insights, models.KeyInsight{
			Priority: priority,
			Icon:     icon,
			Message:  message,
		})
	}

	// K/D ratio comparison
	kdDiff := comp.Team1.Stats.KDRatio - comp.Team2.Stats.KDRatio
	if math.Abs(kdDiff) >= 0.15 {
		var message string
		icon := "🟡"
		if kdDiff > 0 {
			message = fmt.Sprintf("You have better K/D ratio (+%.2f) - maintain aggressive plays", kdDiff)
			icon = "🟢"
		} else {
			message = fmt.Sprintf("Opponent has better K/D ratio (+%.2f) - focus on trades", -kdDiff)
		}
		insights = append(insights, models.KeyInsight{
			Priority: "MEDIUM",
			Icon:     icon,
			Message:  message,
		})
	}

	// Data quality warnings (LOW priority)
	if len(comp.Warnings) > 0 {
		for _, warning := range comp.Warnings {
			insights = append(insights, models.KeyInsight{
				Priority: "LOW",
				Icon:     "⚠️",
				Message:  warning,
			})
		}
	}

	// If no high priority insights, add consistency note
	if len(insights) == 0 || !s.hasHighPriorityInsight(insights) {
		insights = append(insights, models.KeyInsight{
			Priority: "LOW",
			Icon:     "🟢",
			Message:  "Both teams showing consistent performance - matchup will be competitive",
		})
	}

	return insights
}

// hasHighPriorityInsight checks if any HIGH priority insights exist
func (s *ReportService) hasHighPriorityInsight(insights []models.KeyInsight) bool {
	for _, insight := range insights {
		if insight.Priority == "HIGH" {
			return true
		}
	}
	return false
}
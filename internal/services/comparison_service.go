package services

import (
	"context"
	"fmt"

	"github.com/yourusername/esports-scouting-backend/internal/grid"
	"github.com/yourusername/esports-scouting-backend/internal/models"
	"github.com/yourusername/esports-scouting-backend/internal/repository"
	"github.com/yourusername/esports-scouting-backend/pkg/cache"
)

type ComparisonService struct {
	gridClient    *grid.Client
	cache         *cache.RedisClient
	pgRepo        *repository.PostgresRepo
	trendsService *TrendsService
}

func NewComparisonService(gc *grid.Client, rc *cache.RedisClient, pg *repository.PostgresRepo) *ComparisonService {
	return &ComparisonService{
		gridClient:    gc,
		cache:         rc,
		pgRepo:        pg,
		trendsService: NewTrendsService(gc, rc),
	}
}

func (s *ComparisonService) CompareTeams(ctx context.Context, team1Name, team2Name, title string, timeWindow models.TimeWindow, tournamentIDs []string) (*models.ComparisonReport, error) {
	// Get stats directly by team name (no need for FindTeamByName)
	stats1, err1 := s.gridClient.GetTeamStatistics(ctx, team1Name, title, timeWindow, tournamentIDs)
	if err1 != nil {
		return nil, fmt.Errorf("failed to fetch stats for %s: %w", team1Name, err1)
	}

	stats2, err2 := s.gridClient.GetTeamStatistics(ctx, team2Name, title, timeWindow, tournamentIDs)
	if err2 != nil {
		return nil, fmt.Errorf("failed to fetch stats for %s: %w", team2Name, err2)
	}

	// Calculate confidence scores
	stats1.Confidence = CalculateConfidence(stats1.SampleSize, stats1.MatchesPlayed, timeWindow)
	stats2.Confidence = CalculateConfidence(stats2.SampleSize, stats2.MatchesPlayed, timeWindow)

	// Generate warnings based on confidence levels
	warnings := GenerateWarnings(team1Name, stats1.Confidence, team2Name, stats2.Confidence)

	// Build report
	report := &models.ComparisonReport{
		Team1: models.ComparisonTeamData{
			Name:  team1Name,
			Stats: s.buildComparisonStats(stats1),
		},
		Team2: models.ComparisonTeamData{
			Name:  team2Name,
			Stats: s.buildComparisonStats(stats2),
		},
		Advantages: models.Advantages{},
		DataQuality: models.DataQuality{
			Team1Matches: stats1.MatchesPlayed,
			Team2Matches: stats2.MatchesPlayed,
			TimeRange:    timeWindow,
		},
		Warnings: warnings,
	}

	s.calculateAdvantages(report)

	// Optionally add recent trends if analyzing longer periods
	if timeWindow == models.Last3Months || timeWindow == models.Last6Months || timeWindow == models.LastYear {
		recentTrends := s.analyzeRecentTrends(ctx, team1Name, team2Name, title, tournamentIDs)
		if recentTrends != nil {
			report.RecentTrends = recentTrends
		}
	}

	return report, nil
}

// buildComparisonStats extracts duplicate code for building comparison stats
func (s *ComparisonService) buildComparisonStats(stats *models.TeamStats) models.ComparisonStats {
	return models.ComparisonStats{
		WinRate: stats.WinRate,
		KDRatio: stats.KDRatio,
		Kills: models.StatVal{
			Avg:   stats.KillsAvg,
			Total: stats.Kills,
		},
		Deaths: models.StatVal{
			Avg:   stats.DeathsAvg,
			Total: stats.Deaths,
		},
		CurrentStreak: stats.CurrentStreak,
		Confidence:    stats.Confidence,
	}
}

func (s *ComparisonService) analyzeRecentTrends(ctx context.Context, team1Name, team2Name, title string, tournamentIDs []string) *models.RecentTrends {
	// Try to get trends for both teams (non-blocking)
	trends1, err1 := s.trendsService.AnalyzeTrends(ctx, team1Name, title, tournamentIDs)
	trends2, err2 := s.trendsService.AnalyzeTrends(ctx, team2Name, title, tournamentIDs)

	// If both fail, don't include trends
	if err1 != nil && err2 != nil {
		return nil
	}

	recentTrends := &models.RecentTrends{}

	if err1 == nil && len(trends1.Alerts) > 0 {
		recentTrends.Team1HasAlerts = true
		recentTrends.Team1Alerts = s.filterSignificantAlerts(trends1.Alerts)
	}

	if err2 == nil && len(trends2.Alerts) > 0 {
		recentTrends.Team2HasAlerts = true
		recentTrends.Team2Alerts = s.filterSignificantAlerts(trends2.Alerts)
	}

	// Only return if at least one team has alerts
	if recentTrends.Team1HasAlerts || recentTrends.Team2HasAlerts {
		return recentTrends
	}

	return nil
}

// filterSignificantAlerts extracts duplicate filtering logic
func (s *ComparisonService) filterSignificantAlerts(alerts []models.TrendAlert) []models.TrendAlert {
	var significantAlerts []models.TrendAlert
	for _, alert := range alerts {
		if alert.Type != models.AlertConsistency || alert.Severity != models.AlertLow {
			significantAlerts = append(significantAlerts, alert)
		}
	}
	return significantAlerts
}

func (s *ComparisonService) calculateAdvantages(report *models.ComparisonReport) {
	wrDiff := report.Team1.Stats.WinRate - report.Team2.Stats.WinRate
	if wrDiff >= 0.05 {
		report.Advantages.Team1 = append(report.Advantages.Team1, fmt.Sprintf("Higher win rate (+%.0f%%)", wrDiff*100))
	} else if wrDiff <= -0.05 {
		report.Advantages.Team2 = append(report.Advantages.Team2, fmt.Sprintf("Higher win rate (+%.0f%%)", -wrDiff*100))
	}

	kdDiff := report.Team1.Stats.KDRatio - report.Team2.Stats.KDRatio
	if kdDiff >= 0.1 {
		report.Advantages.Team1 = append(report.Advantages.Team1, fmt.Sprintf("Better K/D (+%.1f)", kdDiff))
	} else if kdDiff <= -0.1 {
		report.Advantages.Team2 = append(report.Advantages.Team2, fmt.Sprintf("Better K/D (+%.1f)", -kdDiff))
	}

	s1 := report.Team1.Stats.CurrentStreak
	s2 := report.Team2.Stats.CurrentStreak
	if s1.Type == "win" && (s2.Type != "win" || s1.Count > s2.Count) {
		report.Advantages.Team1 = append(report.Advantages.Team1, "Stronger win streak")
	} else if s2.Type == "win" && (s1.Type != "win" || s2.Count > s1.Count) {
		report.Advantages.Team2 = append(report.Advantages.Team2, "Stronger win streak")
	}
}
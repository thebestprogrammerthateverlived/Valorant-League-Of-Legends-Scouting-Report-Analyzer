package services

import (
	"context"
	"fmt"
	"math"

	"github.com/yourusername/esports-scouting-backend/internal/grid"
	"github.com/yourusername/esports-scouting-backend/internal/models"
	"github.com/yourusername/esports-scouting-backend/pkg/cache"
)

type TrendsService struct {
	gridClient *grid.Client
	cache      *cache.RedisClient
}

func NewTrendsService(gc *grid.Client, rc *cache.RedisClient) *TrendsService {
	return &TrendsService{
		gridClient: gc,
		cache:      rc,
	}
}

// AnalyzeTrends compares recent performance to overall baseline
//func (s *TrendsService) AnalyzeTrends(ctx context.Context, teamName, title string, tournamentIDs []string) (*models.TrendReport, error) {
//	// Find the team
//	team, err := s.gridClient.FindTeamByName(ctx, teamName, title)
//	if err != nil {
//		return nil, fmt.Errorf("failed to find team %s: %w", teamName, err)
//	}
//
//	// Fetch overall stats (3 months baseline)
//	overallStats, err := s.gridClient.GetTeamStatistics(ctx, team.ID, title, models.Last3Months, tournamentIDs)
//	if err != nil {
//		return nil, fmt.Errorf("failed to fetch overall stats: %w", err)
//	}
//
//	// Fetch recent stats (last week)
//	recentStats, err := s.gridClient.GetTeamStatistics(ctx, team.ID, title, models.LastWeek, tournamentIDs)
//	if err != nil {
//		return nil, fmt.Errorf("failed to fetch recent stats: %w", err)
//	}
//
//	// Build period stats
//	overall := models.PeriodStats{
//		TimeWindow: models.Last3Months,
//		WinRate:    overallStats.WinRate,
//		KDRatio:    overallStats.KDRatio,
//		Matches:    overallStats.MatchesPlayed,
//	}
//
//	recent := models.PeriodStats{
//		TimeWindow: models.LastWeek,
//		WinRate:    recentStats.WinRate,
//		KDRatio:    recentStats.KDRatio,
//		Matches:    recentStats.MatchesPlayed,
//	}
//
//	// Analyze trends and generate alerts
//	alerts := s.generateAlerts(overall, recent)
//
//	// Calculate confidence for trend analysis
//	confidence := s.calculateTrendConfidence(recent.Matches, overall.Matches)
//
//	return &models.TrendReport{
//		Team:       team.Name,
//		Title:      title,
//		Overall:    overall,
//		Recent:     recent,
//		Alerts:     alerts,
//		Confidence: confidence,
//	}, nil
//}

// AnalyzeTrends compares recent performance to overall baseline
func (s *TrendsService) AnalyzeTrends(ctx context.Context, teamName, title string, tournamentIDs []string) (*models.TrendReport, error) {
	// Fetch overall stats (3 months baseline) - use team NAME, not ID
	overallStats, err := s.gridClient.GetTeamStatistics(ctx, teamName, title, models.Last3Months, tournamentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch overall stats: %w", err)
	}

	// Fetch recent stats (last week)
	recentStats, err := s.gridClient.GetTeamStatistics(ctx, teamName, title, models.LastWeek, tournamentIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recent stats: %w", err)
	}

	// Build period stats
	overall := models.PeriodStats{
		TimeWindow: models.Last3Months,
		WinRate:    overallStats.WinRate,
		KDRatio:    overallStats.KDRatio,
		Matches:    overallStats.MatchesPlayed,
	}

	recent := models.PeriodStats{
		TimeWindow: models.LastWeek,
		WinRate:    recentStats.WinRate,
		KDRatio:    recentStats.KDRatio,
		Matches:    recentStats.MatchesPlayed,
	}

	// Analyze trends and generate alerts
	alerts := s.generateAlerts(overall, recent)

	// Calculate confidence for trend analysis
	confidence := s.calculateTrendConfidence(recent.Matches, overall.Matches)

	return &models.TrendReport{
		Team:       teamName,
		Title:      title,
		Overall:    overall,
		Recent:     recent,
		Alerts:     alerts,
		Confidence: confidence,
	}, nil
}

func (s *TrendsService) generateAlerts(overall, recent models.PeriodStats) []models.TrendAlert {
	var alerts []models.TrendAlert

	// Check if recent sample is too small
	if recent.Matches < 2 {
		alerts = append(alerts, models.TrendAlert{
			Type:     models.AlertConsistency,
			Severity: models.AlertLow,
			Message:  "Insufficient recent data for trend analysis",
			Context:  fmt.Sprintf("Only %d recent match(es) available", recent.Matches),
		})
		return alerts
	}

	// Analyze win rate change
	winRateChange := (recent.WinRate - overall.WinRate) / overall.WinRate
	winRateChangePct := winRateChange * 100

	if math.Abs(winRateChangePct) >= 15 {
		severity := s.determineSeverity(math.Abs(winRateChangePct))
		alertType := models.AlertPositiveShift
		direction := "increased"
		context := "Team is performing significantly better recently"

		if winRateChangePct < 0 {
			alertType = models.AlertNegativeShift
			direction = "decreased"
			context = "Team is underperforming in recent matches"
		}

		alerts = append(alerts, models.TrendAlert{
			Type:     alertType,
			Severity: severity,
			Message:  fmt.Sprintf("Win rate %s by %.0f%% in recent matches", direction, math.Abs(winRateChangePct)),
			Context:  context,
		})
	}

	// Analyze K/D ratio change
	kdChange := (recent.KDRatio - overall.KDRatio) / overall.KDRatio
	kdChangePct := kdChange * 100

	if math.Abs(kdChangePct) >= 10 {
		severity := s.determineSeverity(math.Abs(kdChangePct))
		direction := "improved"
		context := "More aggressive or efficient plays"

		if kdChangePct < 0 {
			direction = "declined"
			context = "Less efficient or more deaths recently"
		}

		alerts = append(alerts, models.TrendAlert{
			Type:     models.AlertPlaystyleChange,
			Severity: severity,
			Message:  fmt.Sprintf("K/D ratio %s by %.0f%%", direction, math.Abs(kdChangePct)),
			Context:  context,
		})
	}

	// Consistency check
	if len(alerts) == 0 && recent.Matches >= 3 {
		alerts = append(alerts, models.TrendAlert{
			Type:     models.AlertConsistency,
			Severity: models.AlertLow,
			Message:  "Performance remains consistent",
			Context:  "No significant changes detected in recent matches",
		})
	}

	return alerts
}

func (s *TrendsService) determineSeverity(changePct float64) models.AlertSeverity {
	if changePct >= 25 {
		return models.AlertHigh
	} else if changePct >= 15 {
		return models.AlertMedium
	}
	return models.AlertLow
}

func (s *TrendsService) calculateTrendConfidence(recentMatches, overallMatches int) models.Confidence {
	var level models.ConfidenceLevel
	var reliabilityScore int
	var reasoning string

	if recentMatches < 3 {
		level = models.ConfidenceLow
		reliabilityScore = 35
		reasoning = fmt.Sprintf("Recent sample is very small (%d matches) - trend may not be reliable", recentMatches)
	} else if recentMatches < 5 {
		level = models.ConfidenceMedium
		reliabilityScore = 65
		reasoning = fmt.Sprintf("Recent sample is small (%d matches) but trend is observable", recentMatches)
	} else {
		level = models.ConfidenceHigh
		reliabilityScore = 85
		reasoning = fmt.Sprintf("Recent sample is adequate (%d matches) - trend is clear", recentMatches)
	}

	return models.Confidence{
		Level:            level,
		SampleSize:       recentMatches,
		Reasoning:        reasoning,
		ReliabilityScore: reliabilityScore,
	}
}

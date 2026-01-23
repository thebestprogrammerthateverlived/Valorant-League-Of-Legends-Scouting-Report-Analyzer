package services

import (
	"fmt"

	"github.com/yourusername/esports-scouting-backend/internal/models"
)

// CalculateConfidence determines confidence level based on sample size and total matches
func CalculateConfidence(sampleSize int, totalTeamMatches int, timeWindow models.TimeWindow) models.Confidence {
	var level models.ConfidenceLevel
	var reliabilityScore int
	var reasoning string

	// Determine confidence level based on thresholds
	if totalTeamMatches >= 18 {
		// Team has significant match history
		if sampleSize >= 18 {
			level = models.ConfidenceHigh
			reliabilityScore = 95
		} else if sampleSize >= 7 {
			level = models.ConfidenceMedium
			reliabilityScore = 70
		} else {
			level = models.ConfidenceLow
			reliabilityScore = 40
		}
	} else {
		// Team has limited match history
		if sampleSize >= 15 {
			level = models.ConfidenceHigh
			reliabilityScore = 90
		} else if sampleSize >= 5 {
			level = models.ConfidenceMedium
			reliabilityScore = 60
		} else {
			level = models.ConfidenceLow
			reliabilityScore = 30
		}
	}

	// Generate reasoning string
	timeWindowStr := formatTimeWindow(timeWindow)
	if sampleSize == totalTeamMatches {
		reasoning = fmt.Sprintf("Based on all %d matches available %s", sampleSize, timeWindowStr)
	} else {
		reasoning = fmt.Sprintf("Based on %d of %d total matches %s", sampleSize, totalTeamMatches, timeWindowStr)
	}

	// Add context about confidence
	switch level {
	case models.ConfidenceHigh:
		reasoning += " - highly reliable predictions"
	case models.ConfidenceMedium:
		reasoning += " - moderately reliable predictions"
	case models.ConfidenceLow:
		reasoning += " - limited data, predictions less reliable"
	}

	return models.Confidence{
		Level:            level,
		SampleSize:       sampleSize,
		Reasoning:        reasoning,
		ReliabilityScore: reliabilityScore,
	}
}

// GenerateWarnings creates warning messages for low-confidence scenarios
func GenerateWarnings(team1Name string, team1Confidence models.Confidence, team2Name string, team2Confidence models.Confidence) []string {
	var warnings []string

	// Check for low sample sizes
	if team1Confidence.Level == models.ConfidenceLow {
		warnings = append(warnings, fmt.Sprintf("%s has low sample size (%d matches) - predictions less reliable", team1Name, team1Confidence.SampleSize))
	}
	if team2Confidence.Level == models.ConfidenceLow {
		warnings = append(warnings, fmt.Sprintf("%s has low sample size (%d matches) - predictions less reliable", team2Name, team2Confidence.SampleSize))
	}

	// Check for very small samples
	if team1Confidence.SampleSize < 3 {
		warnings = append(warnings, fmt.Sprintf("%s has insufficient data (<%d matches) - comparison may not be meaningful", team1Name, 3))
	}
	if team2Confidence.SampleSize < 3 {
		warnings = append(warnings, fmt.Sprintf("%s has insufficient data (<%d matches) - comparison may not be meaningful", team2Name, 3))
	}

	// Check for mismatched confidence levels
	if (team1Confidence.Level == models.ConfidenceHigh && team2Confidence.Level == models.ConfidenceLow) ||
		(team2Confidence.Level == models.ConfidenceHigh && team1Confidence.Level == models.ConfidenceLow) {
		warnings = append(warnings, "Teams have significantly different data quality - comparison may be skewed")
	}

	// Check for large sample size disparity
	if team1Confidence.SampleSize > 0 && team2Confidence.SampleSize > 0 {
		ratio := float64(team1Confidence.SampleSize) / float64(team2Confidence.SampleSize)
		if ratio > 3.0 || ratio < 0.33 {
			warnings = append(warnings, "Teams have significantly different match counts - consider with caution")
		}
	}

	return warnings
}

func formatTimeWindow(tw models.TimeWindow) string {
	switch tw {
	case models.LastWeek:
		return "over the last week"
	case models.LastMonth:
		return "over the last month"
	case models.Last3Months:
		return "over the last 3 months"
	case models.Last6Months:
		return "over the last 6 months"
	case models.LastYear:
		return "over the last year"
	default:
		return "in the selected time period"
	}
}

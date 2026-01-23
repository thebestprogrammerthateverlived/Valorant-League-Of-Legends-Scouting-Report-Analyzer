package services

import (
	"testing"

	"github.com/yourusername/esports-scouting-backend/internal/models"
)

func TestCalculateAdvantages(t *testing.T) {
	s := &ComparisonService{}

	tests := []struct {
		name           string
		report         *models.ComparisonReport
		expectedTeam1  []string
		expectedTeam2  []string
	}{
		{
			name: "Team 1 better in everything",
			report: &models.ComparisonReport{
				Team1: models.ComparisonTeamData{
					Stats: models.ComparisonStats{
						WinRate: 0.8,
						KDRatio: 1.5,
						CurrentStreak: models.Streak{Type: "win", Count: 5},
					},
				},
				Team2: models.ComparisonTeamData{
					Stats: models.ComparisonStats{
						WinRate: 0.5,
						KDRatio: 1.0,
						CurrentStreak: models.Streak{Type: "loss", Count: 1},
					},
				},
			},
			expectedTeam1: []string{"Higher win rate (+30%)", "Better K/D (+0.5)", "Stronger win streak"},
			expectedTeam2: nil,
		},
		{
			name: "Team 2 better in everything",
			report: &models.ComparisonReport{
				Team1: models.ComparisonTeamData{
					Stats: models.ComparisonStats{
						WinRate: 0.5,
						KDRatio: 1.0,
						CurrentStreak: models.Streak{Type: "loss", Count: 1},
					},
				},
				Team2: models.ComparisonTeamData{
					Stats: models.ComparisonStats{
						WinRate: 0.8,
						KDRatio: 1.5,
						CurrentStreak: models.Streak{Type: "win", Count: 5},
					},
				},
			},
			expectedTeam1: nil,
			expectedTeam2: []string{"Higher win rate (+30%)", "Better K/D (+0.5)", "Stronger win streak"},
		},
		{
			name: "Close match",
			report: &models.ComparisonReport{
				Team1: models.ComparisonTeamData{
					Stats: models.ComparisonStats{
						WinRate: 0.61,
						KDRatio: 1.25,
						CurrentStreak: models.Streak{Type: "win", Count: 2},
					},
				},
				Team2: models.ComparisonTeamData{
					Stats: models.ComparisonStats{
						WinRate: 0.60,
						KDRatio: 1.20,
						CurrentStreak: models.Streak{Type: "win", Count: 2},
					},
				},
			},
			expectedTeam1: nil,
			expectedTeam2: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.calculateAdvantages(tt.report)
			
			if len(tt.report.Advantages.Team1) != len(tt.expectedTeam1) {
				t.Errorf("Team1 advantages length mismatch: got %v, want %v", tt.report.Advantages.Team1, tt.expectedTeam1)
			}
			if len(tt.report.Advantages.Team2) != len(tt.expectedTeam2) {
				t.Errorf("Team2 advantages length mismatch: got %v, want %v", tt.report.Advantages.Team2, tt.expectedTeam2)
			}
		})
	}
}

package grid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/yourusername/esports-scouting-backend/internal/models"
)

type FileDownloader struct {
	apiKey     string
	httpClient *http.Client
}

func NewFileDownloader(apiKey string) *FileDownloader {
	return &FileDownloader{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// FileStatus represents the file availability status
type FileStatus struct {
	Files []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Status      string `json:"status"` // "ready", "processing", "match-not-started", etc.
		FileName    string `json:"fileName"`
		FullURL     string `json:"fullURL"`
	} `json:"files"`
}

// DownloadAndParseSeriesData downloads end-state JSON file and parses it into team stats
func (fd *FileDownloader) DownloadAndParseSeriesData(ctx context.Context, seriesID string, title string) (map[string]*models.SeriesStats, error) {
	// Step 1: Check if file is ready using the list endpoint
	listURL := fmt.Sprintf("https://api.grid.gg/file-download/list/%s", seriesID)

	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %w", err)
	}
	req.Header.Set("x-api-key", fd.apiKey)

	resp, err := fd.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to check file status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("series %s not found or no files available", seriesID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("file list check failed with status %d: %s", resp.StatusCode, string(body))
	}

	var fileStatus FileStatus
	if err := json.NewDecoder(resp.Body).Decode(&fileStatus); err != nil {
		return nil, fmt.Errorf("failed to parse file status: %w", err)
	}

	// Check if end-state file is ready
	var endStateReady bool
	for _, file := range fileStatus.Files {
		if strings.Contains(file.ID, "end-state") && file.Status == "ready" {
			endStateReady = true
			break
		}
	}

	if !endStateReady {
		// Check for status messages
		if len(fileStatus.Files) > 0 {
			status := fileStatus.Files[0].Status
			switch status {
			case "match-not-started":
				return nil, fmt.Errorf("series has not started yet")
			case "match-in-progress":
				return nil, fmt.Errorf("series is still in progress")
			case "processing":
				return nil, fmt.Errorf("series data is being processed, try again in a few minutes")
			case "file-not-available":
				return nil, fmt.Errorf("no data available for this series")
			}
		}
		return nil, fmt.Errorf("end-state file not ready for series %s", seriesID)
	}

	// Step 2: Download the end-state file
	downloadURL := fmt.Sprintf("https://api.grid.gg/file-download/end-state/grid/series/%s", seriesID)

	downloadReq, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}
	downloadReq.Header.Set("x-api-key", fd.apiKey)

	downloadResp, err := fd.httpClient.Do(downloadReq)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(downloadResp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", downloadResp.StatusCode, string(body))
	}

	// Step 3: Parse the JSON end-state file
	return fd.parseEndState(downloadResp.Body)
}

// parseEndState parses the end-state JSON format
func (fd *FileDownloader) parseEndState(reader io.Reader) (map[string]*models.SeriesStats, error) {
	var endState map[string]interface{}
	if err := json.NewDecoder(reader).Decode(&endState); err != nil {
		return nil, fmt.Errorf("failed to parse end-state JSON: %w", err)
	}

	teamStats := make(map[string]*models.SeriesStats)

	// The end-state structure varies by game, but typically has a teams array
	teams, ok := endState["teams"].([]interface{})
	if !ok {
		// Try alternative structure
		if games, ok := endState["games"].([]interface{}); ok {
			return fd.parseFromGames(games)
		}
		return nil, fmt.Errorf("unexpected end-state format: no teams or games array")
	}

	for _, t := range teams {
		teamData, ok := t.(map[string]interface{})
		if !ok {
			continue
		}

		teamID := fd.getString(teamData, "id")
		teamName := fd.getString(teamData, "name")

		if teamID == "" {
			continue
		}

		stats := &models.SeriesStats{
			TeamID:   teamID,
			TeamName: teamName,
		}

		// Extract stats based on available fields
		if outcome, ok := teamData["outcome"].(string); ok {
			stats.Won = outcome == "win"
		}

		if score, ok := teamData["score"].(float64); ok {
			stats.GamesPlayed = int(score)
			if stats.Won {
				stats.Wins = int(score)
			}
		}

		// Try to get kill/death stats
		if players, ok := teamData["players"].([]interface{}); ok {
			for _, p := range players {
				playerData, ok := p.(map[string]interface{})
				if !ok {
					continue
				}

				if kills, ok := playerData["kills"].(float64); ok {
					stats.Kills += int(kills)
				}
				if deaths, ok := playerData["deaths"].(float64); ok {
					stats.Deaths += int(deaths)
				}
				if assists, ok := playerData["assists"].(float64); ok {
					stats.Assists += int(assists)
				}
			}
		}

		// Calculate averages
		if stats.GamesPlayed > 0 {
			stats.KillsAvg = float64(stats.Kills) / float64(stats.GamesPlayed)
			stats.DeathsAvg = float64(stats.Deaths) / float64(stats.GamesPlayed)
			if stats.Deaths > 0 {
				stats.KDRatio = float64(stats.Kills) / float64(stats.Deaths)
			}
		}

		teamStats[teamID] = stats
	}

	if len(teamStats) == 0 {
		return nil, fmt.Errorf("no team stats found in end-state file")
	}

	return teamStats, nil
}

// parseFromGames handles alternative end-state format with games array
func (fd *FileDownloader) parseFromGames(games []interface{}) (map[string]*models.SeriesStats, error) {
	teamStats := make(map[string]*models.SeriesStats)

	for _, g := range games {
		gameData, ok := g.(map[string]interface{})
		if !ok {
			continue
		}

		teams, ok := gameData["teams"].([]interface{})
		if !ok {
			continue
		}

		for _, t := range teams {
			teamData, ok := t.(map[string]interface{})
			if !ok {
				continue
			}

			teamID := fd.getString(teamData, "id")
			teamName := fd.getString(teamData, "name")

			if teamID == "" {
				continue
			}

			if _, exists := teamStats[teamID]; !exists {
				teamStats[teamID] = &models.SeriesStats{
					TeamID:   teamID,
					TeamName: teamName,
				}
			}

			stats := teamStats[teamID]
			stats.GamesPlayed++

			if won, ok := teamData["won"].(bool); ok && won {
				stats.Wins++
				stats.Won = true
			}

			// Aggregate kills/deaths
			if kills, ok := teamData["kills"].(float64); ok {
				stats.Kills += int(kills)
			}
			if deaths, ok := teamData["deaths"].(float64); ok {
				stats.Deaths += int(deaths)
			}
		}
	}

	// Calculate averages
	for _, stats := range teamStats {
		if stats.GamesPlayed > 0 {
			stats.KillsAvg = float64(stats.Kills) / float64(stats.GamesPlayed)
			stats.DeathsAvg = float64(stats.Deaths) / float64(stats.GamesPlayed)
			if stats.Deaths > 0 {
				stats.KDRatio = float64(stats.Kills) / float64(stats.Deaths)
			}
		}
	}

	return teamStats, nil
}

// getString safely extracts string from map
func (fd *FileDownloader) getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}
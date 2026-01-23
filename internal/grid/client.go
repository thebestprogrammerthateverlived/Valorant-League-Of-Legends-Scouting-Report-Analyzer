package grid

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/machinebox/graphql"
	"github.com/yourusername/esports-scouting-backend/internal/models"
)

// TeamNotFoundError indicates team has no data in the available tournaments
type TeamNotFoundError struct {
	TeamName       string
	AvailableTeams []string
}

func (e *TeamNotFoundError) Error() string {
	return fmt.Sprintf("team '%s' did not play in the available tournaments", e.TeamName)
}

type Client struct {
	gqlClient   *graphql.Client
	statsClient *graphql.Client
	apiKey      string
}

// InsufficientDataError indicates team exists but data is unavailable
type InsufficientDataError struct {
	TeamName   string
	Reason     string
	LastMatch  time.Time
}

func (e *InsufficientDataError) Error() string {
	if !e.LastMatch.IsZero() {
		return fmt.Sprintf("insufficient data for team '%s': %s (last match: %s)",
			e.TeamName, e.Reason, e.LastMatch.Format("2006-01-02"))
	}
	return fmt.Sprintf("insufficient data for team '%s': %s", e.TeamName, e.Reason)
}

func NewClient(apiKey string) *Client {
	centralClient := graphql.NewClient("https://api-op.grid.gg/central-data/graphql")
	statsClient := graphql.NewClient("https://api-op.grid.gg/live-data-feed/series-state/graphql") // ← FIXED URL

	return &Client{
		gqlClient:   centralClient,
		statsClient: statsClient,
		apiKey:      apiKey,
	}
}

func (c *Client) newRequest(query string) *graphql.Request {
	req := graphql.NewRequest(query)
	req.Header.Set("X-API-Key", c.apiKey)
	return req
}

// GetTeamSeriesHistory fetches series for a team from hackathon tournaments
func (c *Client) GetTeamSeriesHistory(ctx context.Context, teamIDOrName string, limit int, tournamentIDs []string) ([]SeriesData, error) {
	now := time.Now()
	twoYearsAgo := now.AddDate(-2, 0, 0)

	// Hackathon data: query ALL recent series and filter client-side (max 50 per page)
	var query string
	if len(tournamentIDs) > 0 {
		// Filter by specific tournaments
		query = `
			query($startTime: String!, $tournamentIds: [ID!]) {
				allSeries(
					filter: {
						startTimeScheduled: { gte: $startTime }
						tournament: { id: { in: $tournamentIds }, includeChildren: { equals: true } }
						types: ESPORTS
					}
					orderBy: StartTimeScheduled
					orderDirection: DESC
					first: 50
				) {
					totalCount
					edges {
						node {
							id
							startTimeScheduled
							teams {
								baseInfo {
									id
									name
								}
								scoreAdvantage
							}
						}
					}
				}
			}
		`
	} else {
		// Query all series
		query = `
			query($startTime: String!) {
				allSeries(
					filter: {
						startTimeScheduled: { gte: $startTime }
						types: ESPORTS
					}
					orderBy: StartTimeScheduled
					orderDirection: DESC
					first: 50
				) {
					totalCount
					edges {
						node {
							id
							startTimeScheduled
							teams {
								baseInfo {
									id
									name
								}
								scoreAdvantage
							}
						}
					}
				}
			}
		`
	}

	req := c.newRequest(query)
	req.Var("startTime", twoYearsAgo.Format(time.RFC3339))
	if len(tournamentIDs) > 0 {
		req.Var("tournamentIds", tournamentIDs)
	}

	var resp struct {
		AllSeries struct {
			TotalCount int `json:"totalCount"`
			Edges      []struct {
				Node struct {
					ID                 string    `json:"id"`
					StartTimeScheduled time.Time `json:"startTimeScheduled"`
					Teams              []struct {
						BaseInfo struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						} `json:"baseInfo"`
						ScoreAdvantage int `json:"scoreAdvantage"`
					} `json:"teams"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"allSeries"`
	}

	if err := c.gqlClient.Run(ctx, req, &resp); err != nil {
		fmt.Printf("[DEBUG] GetTeamSeriesHistory error: %v\n", err)
		return nil, fmt.Errorf("failed to fetch series: %w", err)
	}

	fmt.Printf("[DEBUG] Found %d total series from Grid API\n", resp.AllSeries.TotalCount)
	fmt.Printf("[DEBUG] Searching for team by name: %s\n", teamIDOrName)

	// Filter client-side for the specific team by name
	var seriesData []SeriesData
	searchName := strings.ToLower(teamIDOrName)

	for _, edge := range resp.AllSeries.Edges {
		series := edge.Node

		var teamFound bool
		var teamWon bool
		var opponentName string
		var teamID string
		var ourTeamScore, opponentScore int

		for _, team := range series.Teams {
			teamNameLower := strings.ToLower(team.BaseInfo.Name)
			// Match by partial name (e.g., "vitality" matches "Team Vitality")
			if strings.Contains(teamNameLower, searchName) {
				teamFound = true
				teamID = team.BaseInfo.ID
				ourTeamScore = team.ScoreAdvantage
			} else {
				opponentName = team.BaseInfo.Name
				opponentScore = team.ScoreAdvantage
			}
		}

		if teamFound && len(seriesData) < limit {
			teamWon = ourTeamScore > opponentScore
			seriesData = append(seriesData, SeriesData{
				ID:       series.ID,
				TeamID:   teamID,
				Date:     series.StartTimeScheduled,
				Format:   "BO3", // Default
				Won:      teamWon,
				Opponent: opponentName,
			})
		}
	}

	fmt.Printf("[DEBUG] Filtered to %d series for team '%s' (out of %d total)\n", len(seriesData), teamIDOrName, resp.AllSeries.TotalCount)

	if len(seriesData) == 0 {
		// Collect available teams
		teamSet := make(map[string]bool)
		for _, edge := range resp.AllSeries.Edges {
			for _, team := range edge.Node.Teams {
				teamSet[team.BaseInfo.Name] = true
			}
		}

		var availableTeams []string
		for teamName := range teamSet {
			availableTeams = append(availableTeams, teamName)
			if len(availableTeams) >= 30 {
				break
			}
		}

		fmt.Printf("[DEBUG] No series found for '%s'. Sample of available teams: %v\n", teamIDOrName, availableTeams)
		return nil, &TeamNotFoundError{
			TeamName:       teamIDOrName,
			AvailableTeams: availableTeams,
		}
	}

	return seriesData, nil
}

type SeriesData struct {
	ID       string
	TeamID   string
	Date     time.Time
	Format   string
	Won      bool
	Opponent string
}

// GetTeamStatistics fetches series and uses Series State API for detailed stats
// FIXED: Implements graduated fallback for better accuracy
func (c *Client) GetTeamStatistics(ctx context.Context, teamName string, title string, timeWindow models.TimeWindow, tournamentIDs []string) (*models.TeamStats, error) {
	// Auto-select tournaments if none specified
	if len(tournamentIDs) == 0 {
		switch strings.ToLower(title) {
		case "valorant":
			tournamentIDs = []string{
				"757371", "757481", "774782", // 2024
				"775516", "800675", "826660", // 2025
			}
			fmt.Printf("[DEBUG] Auto-selected Valorant tournaments\n")
		case "lol", "leagueoflegends":
			tournamentIDs = []string{
				"758024", "774794", "825490", "826679", // LCK
				"758043", "774888", // LCS
				"758077", "774622", "825468", "826906", // LEC
				"758054", "774845", "775662", "825450", // LPL
			}
			fmt.Printf("[DEBUG] Auto-selected LoL tournaments\n")
		default:
			return nil, fmt.Errorf("no tournaments configured for title: %s", title)
		}
	}

	// Step 1: Get series IDs for this team by name
	seriesHistory, err := c.GetTeamSeriesHistory(ctx, teamName, 50, tournamentIDs)
	if err != nil {
		return nil, err
	}

	if len(seriesHistory) == 0 {
		return nil, fmt.Errorf("no match data found for team %s", teamName)
	}

	// Step 2: Filter by time window with GRADUATED FALLBACK
	now := time.Now()
	var filteredSeries []SeriesData
	actualWindow := timeWindow

	windowSequence := getWindowFallbackSequence(timeWindow)

	for _, window := range windowSequence {
		cutoffDate := calculateCutoffDate(now, window)
		filteredSeries = nil

		for _, series := range seriesHistory {
			if series.Date.After(cutoffDate) {
				filteredSeries = append(filteredSeries, series)
			}
		}

		if len(filteredSeries) >= 3 {
			actualWindow = window
			if window != timeWindow {
				fmt.Printf("[INFO] Expanded time window from %s to %s to get sufficient data (%d matches)\n",
					timeWindow, window, len(filteredSeries))
			}
			break
		}

		fmt.Printf("[DEBUG] %s: found %d matches (need ≥3), trying next window\n", window, len(filteredSeries))
	}

	// ✅ FIX: Return InsufficientDataError instead of generic error
	if len(filteredSeries) == 0 {
		return nil, &InsufficientDataError{
			TeamName:  teamName,
			Reason:    "no recent matches found",
			LastMatch: seriesHistory[0].Date,
		}
	}

	if len(filteredSeries) < 3 {
		fmt.Printf("[WARN] Only %d matches found for %s - confidence will be LOW\n", len(filteredSeries), teamName)
	}

	fmt.Printf("[DEBUG] Using %d series from %s window for stats calculation\n", len(filteredSeries), actualWindow)

	// Step 3: Fetch Series State data
	var totalKills, totalDeaths, totalGames int
	successfulDownloads := 0

	for i, series := range filteredSeries {
		if i >= 10 {
			break
		}

		seriesDataMap, err := c.GetSeriesStats(ctx, series.ID)
		if err != nil {
			fmt.Printf("[DEBUG] Failed to download series %s: %v\n", series.ID, err)
			continue
		}

		searchName := strings.ToLower(teamName)
		foundStats := false
		for _, stats := range seriesDataMap {
			if strings.Contains(strings.ToLower(stats.TeamName), searchName) {
				totalKills += stats.Kills
				totalDeaths += stats.Deaths
				totalGames += stats.GamesPlayed
				successfulDownloads++
				foundStats = true
				fmt.Printf("[DEBUG] Series %s: +%d kills, +%d deaths, +%d games\n",
					series.ID, stats.Kills, stats.Deaths, stats.GamesPlayed)
				break
			}
		}

		if !foundStats {
			fmt.Printf("[WARN] Team %s not found in series %s data\n", teamName, series.ID)
		}
	}

	// Require at least some successful downloads
	if successfulDownloads == 0 {
		return nil, fmt.Errorf("no detailed stats available for team %s - series may not be finished or data unavailable", teamName)
	}

	// ✅ FIX: Return InsufficientDataError instead of generic error
	if successfulDownloads == 0 {
		return nil, &InsufficientDataError{
			TeamName: teamName,
			Reason:   "series data not finished or unavailable",
		}
	}

	totalWins := 0
	for _, series := range filteredSeries {
		if series.Won {
			totalWins++
		}
	}

	totalMatches := len(filteredSeries)
	winRate := float64(totalWins) / float64(totalMatches)

	// Calculate K/D - fail if we don't have real data
	var killsAvg, deathsAvg, kdRatio float64
	if totalGames > 0 && totalDeaths > 0 {
		killsAvg = float64(totalKills) / float64(totalGames)
		deathsAvg = float64(totalDeaths) / float64(totalGames)
		kdRatio = float64(totalKills) / float64(totalDeaths)
	} else {
		return nil, fmt.Errorf("insufficient detailed statistics available for team %s", teamName)
	}

	// Calculate current streak
	streakType := "loss"
	streakCount := 0
	if len(filteredSeries) > 0 {
		if filteredSeries[0].Won {
			streakType = "win"
		}
		for _, series := range filteredSeries {
			if (streakType == "win" && series.Won) || (streakType == "loss" && !series.Won) {
				streakCount++
			} else {
				break
			}
		}
	}

	stats := &models.TeamStats{
		WinRate:       winRate,
		MatchesPlayed: totalMatches,
		Kills:         totalKills,
		KillsAvg:      killsAvg,
		Deaths:        totalDeaths,
		DeathsAvg:     deathsAvg,
		KDRatio:       kdRatio,
		CurrentStreak: models.Streak{
			Type:  streakType,
			Count: streakCount,
		},
		SampleSize: totalMatches,
		// Store actual window used for transparency
		ActualTimeWindow: actualWindow,
	}

	fmt.Printf("[SUCCESS] Retrieved stats from %d/%d series attempts\n", successfulDownloads, min(10, len(filteredSeries)))

	return stats, nil
}

// Helper: Get fallback sequence based on requested window
func getWindowFallbackSequence(requested models.TimeWindow) []models.TimeWindow {
	switch requested {
	case models.LastWeek:
		return []models.TimeWindow{
			models.LastWeek,
			models.LastMonth,      // Fallback 1: expand to month
			models.Last3Months,    // Fallback 2: expand to 3 months
		}
	case models.LastMonth:
		return []models.TimeWindow{
			models.LastMonth,
			models.Last3Months,    // Fallback 1: expand to 3 months
			models.Last6Months,    // Fallback 2: expand to 6 months
		}
	case models.Last3Months:
		return []models.TimeWindow{
			models.Last3Months,
			models.Last6Months,    // Fallback 1: expand to 6 months
			models.LastYear,       // Fallback 2: expand to year
		}
	case models.Last6Months:
		return []models.TimeWindow{
			models.Last6Months,
			models.LastYear,       // Only fallback: expand to year
		}
	case models.LastYear:
		return []models.TimeWindow{
			models.LastYear,       // No fallback - use all available data
		}
	default:
		return []models.TimeWindow{models.Last3Months}
	}
}

// Helper: Calculate cutoff date for a time window
func calculateCutoffDate(now time.Time, window models.TimeWindow) time.Time {
	switch window {
	case models.LastWeek:
		return now.AddDate(0, 0, -7)
	case models.LastMonth:
		return now.AddDate(0, -1, 0)
	case models.Last3Months:
		return now.AddDate(0, -3, 0)
	case models.Last6Months:
		return now.AddDate(0, -6, 0)
	case models.LastYear:
		return now.AddDate(-1, 0, 0)
	default:
		return now.AddDate(0, -3, 0)
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) HealthCheck(ctx context.Context) bool {
	query := `{ __schema { types { name } } }`
	req := c.newRequest(query)
	var resp interface{}
	err := c.gqlClient.Run(ctx, req, &resp)
	if err != nil {
		fmt.Printf("[DEBUG] HealthCheck failed: %v\n", err)
	}
	return err == nil
}

// GetAvailableTeams fetches all unique team names from recent series
func (c *Client) GetAvailableTeams(ctx context.Context, title string, tournamentIDs []string) ([]string, error) {
	now := time.Now()
	twoYearsAgo := now.AddDate(-2, 0, 0)

	// Convert title to titleID
	titleID := ""
	switch strings.ToLower(title) {
	case "valorant":
		titleID = "6"
	case "lol", "leagueoflegends":
		titleID = "3"
	case "r6", "rainbow6", "siege":
		titleID = "25"
	default:
		titleID = title
	}

	fmt.Printf("[DEBUG] GetAvailableTeams - Title: %s (ID: %s), TournamentIDs: %v\n", title, titleID, tournamentIDs)

	// Auto-select tournaments if none specified (same logic as GetTeamStatistics)
	if len(tournamentIDs) == 0 {
		switch strings.ToLower(title) {
		case "valorant":
			tournamentIDs = []string{
				"757371", "757481", "774782", // 2024
				"775516", "800675", "826660", // 2025
			}
			fmt.Printf("[DEBUG] Auto-selected Valorant tournaments: %v\n", tournamentIDs)
		case "lol", "leagueoflegends":
			tournamentIDs = []string{
				"758024", "774794", "825490", "826679", // LCK
				"758043", "774888", // LCS
				"758077", "774622", "825468", "826906", // LEC
				"758054", "774845", "775662", "825450", // LPL
			}
			fmt.Printf("[DEBUG] Auto-selected LoL tournaments: %v\n", tournamentIDs)
		}
	}

	// Now tournamentIDs will always be set for valorant/lol
	var query string
	if len(tournamentIDs) > 0 {
		query = `
			query($startTime: String!, $tournamentIds: [ID!]!) {
				allSeries(
					filter: {
						startTimeScheduled: { gte: $startTime }
						tournament: { id: { in: $tournamentIds }, includeChildren: { equals: true } }
						types: ESPORTS
					}
					orderBy: StartTimeScheduled
					orderDirection: DESC
					first: 50
				) {
					edges {
						node {
							id
							title {
								id
								name
							}
							teams {
								baseInfo {
									name
								}
							}
						}
					}
				}
			}
		`
	} else {
		query = `
			query($startTime: String!) {
				allSeries(
					filter: {
						startTimeScheduled: { gte: $startTime }
						types: ESPORTS
					}
					orderBy: StartTimeScheduled
					orderDirection: DESC
					first: 50
				) {
					edges {
						node {
							id
							title {
								id
								name
							}
							teams {
								baseInfo {
									name
								}
							}
						}
					}
				}
			}
		`
	}

	req := c.newRequest(query)
	req.Var("startTime", twoYearsAgo.Format(time.RFC3339))
	if len(tournamentIDs) > 0 {
		req.Var("tournamentIds", tournamentIDs)
	}

	var resp struct {
		AllSeries struct {
			Edges []struct {
				Node struct {
					ID    string `json:"id"`
					Title struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"title"`
					Teams []struct {
						BaseInfo struct {
							Name string `json:"name"`
						} `json:"baseInfo"`
					} `json:"teams"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"allSeries"`
	}

	if err := c.gqlClient.Run(ctx, req, &resp); err != nil {
		fmt.Printf("[ERROR] GetAvailableTeams GraphQL error: %v\n", err)
		return nil, fmt.Errorf("failed to fetch teams: %w", err)
	}

	fmt.Printf("[DEBUG] Fetched %d series total\n", len(resp.AllSeries.Edges))

	// Extract team names
	teamSet := make(map[string]bool)
	for _, edge := range resp.AllSeries.Edges {
		for _, team := range edge.Node.Teams {
			if team.BaseInfo.Name != "" {
				teamSet[team.BaseInfo.Name] = true
			}
		}
	}

	// Convert to slice
	teams := make([]string, 0, len(teamSet))
	for teamName := range teamSet {
		teams = append(teams, teamName)
	}

	fmt.Printf("[DEBUG] Found %d unique teams\n", len(teams))

	return teams, nil
}

// ✅ NEW: GetAvailableTeamsWithData - Only returns teams with accessible Series State data
func (c *Client) GetAvailableTeamsWithData(ctx context.Context, title string, tournamentIDs []string) ([]string, error) {
	// Auto-select tournaments
	if len(tournamentIDs) == 0 {
		switch strings.ToLower(title) {
		case "valorant":
			tournamentIDs = []string{
				"757371", "757481", "774782",
				"775516", "800675", "826660",
			}
		case "lol", "leagueoflegends":
			tournamentIDs = []string{
				"758024", "774794", "825490", "826679",
				"758043", "774888",
				"758077", "774622", "825468", "826906",
				"758054", "774845", "775662", "825450",
			}
		}
	}

	// Get all series
	now := time.Now()
	twoYearsAgo := now.AddDate(-2, 0, 0)

	query := `
		query($startTime: String!, $tournamentIds: [ID!]!) {
			allSeries(
				filter: {
					startTimeScheduled: { gte: $startTime }
					tournament: { id: { in: $tournamentIds }, includeChildren: { equals: true } }
					types: ESPORTS
				}
				orderBy: StartTimeScheduled
				orderDirection: DESC
				first: 50
			) {
				edges {
					node {
						id
						startTimeScheduled
						teams {
							baseInfo { name }
						}
					}
				}
			}
		}
	`

	req := c.newRequest(query)
	req.Var("startTime", twoYearsAgo.Format(time.RFC3339))
	req.Var("tournamentIds", tournamentIDs)

	var resp struct {
		AllSeries struct {
			Edges []struct {
				Node struct {
					ID                 string    `json:"id"`
					StartTimeScheduled time.Time `json:"startTimeScheduled"`
					Teams              []struct {
						BaseInfo struct {
							Name string `json:"name"`
						} `json:"baseInfo"`
					} `json:"teams"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"allSeries"`
	}

	if err := c.gqlClient.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("failed to fetch series: %w", err)
	}

	// Track teams that have Series State data
	teamsWithData := make(map[string]bool)
	teamSeriesMap := make(map[string][]string) // team -> series IDs

	// Build map of team -> series
	for _, edge := range resp.AllSeries.Edges {
		for _, team := range edge.Node.Teams {
			if team.BaseInfo.Name != "" {
				teamSeriesMap[team.BaseInfo.Name] = append(teamSeriesMap[team.BaseInfo.Name], edge.Node.ID)
			}
		}
	}

	fmt.Printf("[DEBUG] Validating data access for %d teams...\n", len(teamSeriesMap))

	// Check each team - sample 1 series to verify data access
	for teamName, seriesIDs := range teamSeriesMap {
		if len(seriesIDs) == 0 {
			continue
		}

		// Try the most recent series
		seriesID := seriesIDs[0]
		_, err := c.GetSeriesStats(ctx, seriesID)
		if err == nil {
			teamsWithData[teamName] = true
			fmt.Printf("[DEBUG] ✓ %s has data access\n", teamName)
		} else {
			fmt.Printf("[DEBUG] ✗ %s lacks data access: %v\n", teamName, err)
		}
	}

	// Convert to slice
	teams := make([]string, 0, len(teamsWithData))
	for teamName := range teamsWithData {
		teams = append(teams, teamName)
	}

	fmt.Printf("[DEBUG] Found %d teams with accessible data (out of %d total)\n", len(teams), len(teamSeriesMap))

	return teams, nil
}

// GetSeriesStats fetches detailed stats for a series using Series State API
func (c *Client) GetSeriesStats(ctx context.Context, seriesID string) (map[string]*models.SeriesStats, error) {
	query := `
		query($seriesId: ID!) {
			seriesState(id: $seriesId) {
				id
				started
				finished
				teams {
					id
					name
					won
				}
				games {
					teams {
						id
						name
						players {
							id
							name
							kills
							deaths
						}
					}
				}
			}
		}
	`

	req := c.newRequest(query)
	req.Var("seriesId", seriesID)

	var resp struct {
		SeriesState struct {
			ID       string `json:"id"`
			Started  bool   `json:"started"`
			Finished bool   `json:"finished"`
			Teams    []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Won  bool   `json:"won"`
			} `json:"teams"`
			Games []struct {
				Teams []struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Players []struct {
						ID     string `json:"id"`
						Name   string `json:"name"`
						Kills  int    `json:"kills"`
						Deaths int    `json:"deaths"`
					} `json:"players"`
				} `json:"teams"`
			} `json:"games"`
		} `json:"seriesState"`
	}

	if err := c.statsClient.Run(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("series state API error: %w", err)
	}

	if !resp.SeriesState.Finished {
		return nil, fmt.Errorf("series has not finished yet")
	}

	// Aggregate stats per team
	teamStats := make(map[string]*models.SeriesStats)

	// Initialize teams
	for _, team := range resp.SeriesState.Teams {
		teamStats[team.ID] = &models.SeriesStats{
			SeriesID:    seriesID,
			TeamID:      team.ID,
			TeamName:    team.Name,
			Won:         team.Won,
			GamesPlayed: len(resp.SeriesState.Games),
		}
	}

	// Aggregate kills/deaths from all games
	for _, game := range resp.SeriesState.Games {
		for _, team := range game.Teams {
			if stats, exists := teamStats[team.ID]; exists {
				for _, player := range team.Players {
					stats.Kills += player.Kills
					stats.Deaths += player.Deaths
				}
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
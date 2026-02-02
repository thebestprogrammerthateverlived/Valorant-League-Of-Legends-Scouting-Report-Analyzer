package models

import "time"

type TimeWindow string

const (
	LastWeek    TimeWindow = "LAST_WEEK"
	LastMonth   TimeWindow = "LAST_MONTH"
	Last3Months TimeWindow = "LAST_3_MONTHS"
	Last6Months TimeWindow = "LAST_6_MONTHS"
	LastYear    TimeWindow = "LAST_YEAR"
)

type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "HIGH"
	ConfidenceMedium ConfidenceLevel = "MEDIUM"
	ConfidenceLow    ConfidenceLevel = "LOW"
)

type Confidence struct {
	Level            ConfidenceLevel `json:"level"`
	SampleSize       int             `json:"sampleSize"`
	Reasoning        string          `json:"reasoning"`
	ReliabilityScore int             `json:"reliabilityScore"` // 0-100
}

type Team struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
}

type CommonStats struct {
	Kills      int     `json:"kills"`
	Deaths     int     `json:"deaths"`
	Assists    int     `json:"assists"`
	WinRate    float64 `json:"winRate"`
	SampleSize int     `json:"sampleSize"`
}

type TeamStats struct {
	WinRate          float64    `json:"winRate"`
	KDRatio          float64    `json:"kdRatio"`
	MatchesPlayed    int        `json:"matchesPlayed"`
	Kills            int        `json:"kills"`
	Deaths           int        `json:"deaths"`
	Assists          int        `json:"assists"`
	KillsAvg         float64    `json:"killsAvg"`
	DeathsAvg        float64    `json:"deathsAvg"`
	AssistsAvg       float64    `json:"assistsAvg"`
	CurrentStreak    Streak     `json:"currentStreak"`
	SampleSize       int        `json:"sampleSize"`
	Confidence       Confidence `json:"confidence"`
	ActualTimeWindow TimeWindow `json:"actualTimeWindow,omitempty"` // ✅ ADDED
}

type PlayerStats struct {
	TeamStats
}

type Streak struct {
	Type  string `json:"type"` // "win" or "loss"
	Count int    `json:"count"`
}

type ComparisonReport struct {
	Team1        ComparisonTeamData `json:"team1"`
	Team2        ComparisonTeamData `json:"team2"`
	Advantages   Advantages         `json:"advantages"`
	DataQuality  DataQuality        `json:"dataQuality"`
	Warnings     []string           `json:"warnings,omitempty"`
	RecentTrends *RecentTrends      `json:"recentTrends,omitempty"`
}

type ComparisonTeamData struct {
	Name  string          `json:"name"`
	Stats ComparisonStats `json:"stats"`
}

type ComparisonStats struct {
	WinRate       float64    `json:"winRate"`
	KDRatio       float64    `json:"kdRatio"`
	Kills         StatVal    `json:"kills"`
	Deaths        StatVal    `json:"deaths"`
	CurrentStreak Streak     `json:"currentStreak"`
	Confidence    Confidence `json:"confidence"`
}

type StatVal struct {
	Avg   float64 `json:"avg"`
	Total int     `json:"total"`
}

type Advantages struct {
	Team1 []string `json:"team1"`
	Team2 []string `json:"team2"`
}

type DataQuality struct {
	Team1Matches int        `json:"team1Matches"`
	Team2Matches int        `json:"team2Matches"`
	TimeRange    TimeWindow `json:"timeRange"`
}

type PickRates map[string]float64

// Trend Analysis Models
type AlertSeverity string

const (
	AlertHigh   AlertSeverity = "HIGH"
	AlertMedium AlertSeverity = "MEDIUM"
	AlertLow    AlertSeverity = "LOW"
)

type AlertType string

const (
	AlertPositiveShift   AlertType = "POSITIVE_SHIFT"
	AlertNegativeShift   AlertType = "NEGATIVE_SHIFT"
	AlertPlaystyleChange AlertType = "PLAYSTYLE_CHANGE"
	AlertConsistency     AlertType = "CONSISTENCY"
)

type TrendAlert struct {
	Type     AlertType     `json:"type"`
	Severity AlertSeverity `json:"severity"`
	Message  string        `json:"message"`
	Context  string        `json:"context"`
}

type PeriodStats struct {
	TimeWindow TimeWindow `json:"timeWindow"`
	WinRate    float64    `json:"winRate"`
	KDRatio    float64    `json:"kdRatio"`
	Matches    int        `json:"matches"`
}

type TrendReport struct {
	Team       string       `json:"team"`
	Title      string       `json:"title"`
	Overall    PeriodStats  `json:"overall"`
	Recent     PeriodStats  `json:"recent"`
	Alerts     []TrendAlert `json:"alerts"`
	Confidence Confidence   `json:"confidence"`
}

type RecentTrends struct {
	Team1HasAlerts bool         `json:"team1HasAlerts"`
	Team2HasAlerts bool         `json:"team2HasAlerts"`
	Team1Alerts    []TrendAlert `json:"team1Alerts,omitempty"`
	Team2Alerts    []TrendAlert `json:"team2Alerts,omitempty"`
}

type SeriesRecord struct {
	ID             string
	Team1ID        string
	Team2ID        string
	Team1Name      string
	Team2Name      string
	Title          string
	StartTime      time.Time
	Team1Won       bool
	Format         string
	DataDownloaded bool
}

// JSONL Event structure (simplified for kill/death events)
type GridEvent struct {
	Type       string                 `json:"type"`
	OccurredAt string                 `json:"occurredAt"`
	Payload    map[string]interface{} `json:"payload"`
}

type SeriesStats struct {
	SeriesID    string  `json:"seriesId"` // Added for postgres.go
	TeamID      string  `json:"teamId"`
	TeamName    string  `json:"teamName"`
	GamesPlayed int     `json:"gamesPlayed"`
	Wins        int     `json:"wins"`
	Won         bool    `json:"won"`        // Did the team win this series
	Kills       int     `json:"kills"`      // Renamed from TotalKills
	Deaths      int     `json:"deaths"`     // Renamed from TotalDeaths
	Assists     int     `json:"assists"`    // Added for postgres.go
	RoundsWon   int     `json:"roundsWon"`  // Added for postgres.go
	RoundsLost  int     `json:"roundsLost"` // Added for postgres.go
	KillsAvg    float64 `json:"killsAvg"`
	DeathsAvg   float64 `json:"deathsAvg"`
	KDRatio     float64 `json:"kdRatio"`
}

// FEATURE #7: META ANALYSIS & SCOUTING REPORT MODELS

// MetaPick represents a champion/agent pick with statistics
type MetaPick struct {
	Name        string  `json:"name"`
	PickRate    float64 `json:"pickRate"`
	WinRate     float64 `json:"winRate"`
	Tier        string  `json:"tier"`     // S, A, B, C
	Trending    string  `json:"trending"` // "rising", "stable", "declining"
	GamesPlayed int     `json:"gamesPlayed"`
}

// MetaShift represents changes in the meta
type MetaShift struct {
	Pick   string `json:"pick"`
	Change string `json:"change"`
	Reason string `json:"reason,omitempty"`
}

// MetaReport is the response for /api/v1/meta
type MetaReport struct {
	Title       string      `json:"title"`
	Tournament  string      `json:"tournament,omitempty"`
	TopPicks    []MetaPick  `json:"topPicks"`
	MetaShifts  []MetaShift `json:"metaShifts"`
	GeneratedAt time.Time   `json:"generatedAt"`
	SampleSize  int         `json:"sampleSize"`
}

// MetaContext provides meta-related context for a team
type MetaContext struct {
	OpponentVsMeta  []string `json:"opponentVsMeta"`
	YourTeamVsMeta  []string `json:"yourTeamVsMeta"`
	Recommendations []string `json:"recommendations"`
}

// KeyInsight represents a prioritized insight
type KeyInsight struct {
	Priority string `json:"priority"` // "HIGH", "MEDIUM", "LOW"
	Icon     string `json:"icon"`     // "🔴", "🟡", "🟢"
	Message  string `json:"message"`
}

// CacheStatus tracks cache performance
type CacheStatus struct {
	FromCache bool   `json:"fromCache"`
	Age       string `json:"age"`
}

// ScoutingReport is the comprehensive combined report
type ScoutingReport struct {
	ReportID    string           `json:"reportId"`
	GeneratedAt time.Time        `json:"generatedAt"`
	Matchup     MatchupInfo      `json:"matchup"`
	Comparison  ComparisonReport `json:"comparison"`
	Trends      TrendsInfo       `json:"trends"`
	MetaContext MetaContext      `json:"metaContext,omitempty"`
	KeyInsights []KeyInsight     `json:"keyInsights"`
	Confidence  Confidence       `json:"confidence"`
	CacheStatus CacheStatus      `json:"cacheStatus"`
}

// MatchupInfo describes the teams being compared
type MatchupInfo struct {
	Opponent string `json:"opponent"`
	YourTeam string `json:"yourTeam"`
	Title    string `json:"title"`
}

// TrendsInfo contains trend analysis for both teams
type TrendsInfo struct {
	Opponent TrendReport `json:"opponent"`
	YourTeam TrendReport `json:"yourTeam"`
}

// TeamSearchResult for autocomplete
type TeamSearchResult struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Title       string `json:"title"`
	Relevance   int    `json:"relevance"`
}

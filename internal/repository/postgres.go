package repository

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/yourusername/esports-scouting-backend/internal/models"
)

type PostgresRepo struct {
	DB *sql.DB
}

func NewPostgresRepo(databaseURL string) (*PostgresRepo, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	fmt.Println("Successfully connected to NeonDB (PostgreSQL)")
	return &PostgresRepo{DB: db}, nil
}

func (r *PostgresRepo) HealthCheck() bool {
	err := r.DB.Ping()
	return err == nil
}

// RunMigrations runs the schema migrations
func (r *PostgresRepo) RunMigrations() error {
	schema := `
		CREATE TABLE IF NOT EXISTS series (
			id TEXT PRIMARY KEY,
			team1_id TEXT NOT NULL,
			team2_id TEXT NOT NULL,
			team1_name TEXT NOT NULL,
			team2_name TEXT NOT NULL,
			title TEXT NOT NULL,
			start_time TIMESTAMP NOT NULL,
			team1_won BOOLEAN NOT NULL,
			format TEXT,
			data_downloaded BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS series_stats (
			id SERIAL PRIMARY KEY,
			series_id TEXT NOT NULL,
			team_id TEXT NOT NULL,
			kills INT DEFAULT 0,
			deaths INT DEFAULT 0,
			assists INT DEFAULT 0,
			rounds_won INT DEFAULT 0,
			rounds_lost INT DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(series_id, team_id)
		);

		CREATE INDEX IF NOT EXISTS idx_series_team1 ON series(team1_id);
		CREATE INDEX IF NOT EXISTS idx_series_team2 ON series(team2_id);
		CREATE INDEX IF NOT EXISTS idx_series_title ON series(title);
		CREATE INDEX IF NOT EXISTS idx_series_start_time ON series(start_time);
		CREATE INDEX IF NOT EXISTS idx_stats_team ON series_stats(team_id);
	`

	_, err := r.DB.Exec(schema)
	return err
}

// GetTeamStats retrieves stats from DB
func (r *PostgresRepo) GetTeamStats(teamID, title string, startDate, endDate time.Time) (*models.TeamStats, error) {
	query := `
		SELECT 
			COUNT(DISTINCT s.id) as total_series,
			SUM(CASE WHEN (s.team1_id = $1 AND s.team1_won = true) OR (s.team2_id = $1 AND s.team1_won = false) THEN 1 ELSE 0 END) as wins,
			COALESCE(SUM(ss.kills), 0) as total_kills,
			COALESCE(SUM(ss.deaths), 0) as total_deaths,
			COALESCE(SUM(ss.assists), 0) as total_assists
		FROM series s
		LEFT JOIN series_stats ss ON s.id = ss.series_id AND ss.team_id = $1
		WHERE (s.team1_id = $1 OR s.team2_id = $1)
			AND s.title = $2
			AND s.start_time BETWEEN $3 AND $4
			AND s.data_downloaded = true
	`

	var totalSeries, wins, totalKills, totalDeaths, totalAssists int
	err := r.DB.QueryRow(query, teamID, title, startDate, endDate).Scan(
		&totalSeries, &wins, &totalKills, &totalDeaths, &totalAssists,
	)
	if err != nil {
		return nil, err
	}

	if totalSeries == 0 {
		return nil, fmt.Errorf("no data found")
	}

	winRate := float64(wins) / float64(totalSeries)
	killsAvg := float64(totalKills) / float64(totalSeries)
	deathsAvg := float64(totalDeaths) / float64(totalSeries)
	assistsAvg := float64(totalAssists) / float64(totalSeries)
	kdRatio := 0.0
	if deathsAvg > 0 {
		kdRatio = killsAvg / deathsAvg
	}

	// Calculate streak
	streakQuery := `
		SELECT 
			CASE 
				WHEN s.team1_id = $1 THEN s.team1_won
				ELSE NOT s.team1_won
			END as won
		FROM series s
		WHERE (s.team1_id = $1 OR s.team2_id = $1)
			AND s.title = $2
			AND s.start_time BETWEEN $3 AND $4
		ORDER BY s.start_time DESC
		LIMIT 10
	`

	rows, err := r.DB.Query(streakQuery, teamID, title, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var streak models.Streak
	streakCount := 0
	var lastResult *bool

	for rows.Next() {
		var won bool
		if err := rows.Scan(&won); err != nil {
			continue
		}

		if lastResult == nil {
			lastResult = &won
			streakCount = 1
			if won {
				streak.Type = "win"
			} else {
				streak.Type = "loss"
			}
		} else if *lastResult == won {
			streakCount++
		} else {
			break
		}
	}
	streak.Count = streakCount

	return &models.TeamStats{
		WinRate:       winRate,
		MatchesPlayed: totalSeries,
		Kills:         totalKills,
		KillsAvg:      killsAvg,
		Deaths:        totalDeaths,
		DeathsAvg:     deathsAvg,
		Assists:       totalAssists,
		AssistsAvg:    assistsAvg,
		KDRatio:       kdRatio,
		CurrentStreak: streak,
		SampleSize:    totalSeries,
	}, nil
}

// SaveSeries stores series metadata
func (r *PostgresRepo) SaveSeries(s *models.SeriesRecord) error {
	query := `INSERT INTO series (id, team1_id, team2_id, team1_name, team2_name, title, start_time, team1_won, format, data_downloaded)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET team1_won = EXCLUDED.team1_won, data_downloaded = EXCLUDED.data_downloaded`
	_, err := r.DB.Exec(query, s.ID, s.Team1ID, s.Team2ID, s.Team1Name, s.Team2Name, s.Title, s.StartTime, s.Team1Won, s.Format, s.DataDownloaded)
	return err
}

// SaveSeriesStats stores aggregated stats
func (r *PostgresRepo) SaveSeriesStats(stats *models.SeriesStats) error {
	query := `INSERT INTO series_stats (series_id, team_id, kills, deaths, assists, rounds_won, rounds_lost)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (series_id, team_id) DO UPDATE SET kills = EXCLUDED.kills, deaths = EXCLUDED.deaths, assists = EXCLUDED.assists`
	_, err := r.DB.Exec(query, stats.SeriesID, stats.TeamID, stats.Kills, stats.Deaths, stats.Assists, stats.RoundsWon, stats.RoundsLost)
	return err
}



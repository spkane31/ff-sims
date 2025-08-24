package main

import (
	"backend/internal/config"
	"backend/internal/database"
	"fmt"
	"log"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	// Initialize database
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	fmt.Println("Starting deduplication process...")

	// Deduplicate matchups for 2023
	if err := deduplicateMatchups(2023); err != nil {
		log.Fatalf("Error deduplicating matchups: %v", err)
	}

	// Deduplicate box scores for 2023
	if err := deduplicateBoxScores(2023); err != nil {
		log.Fatalf("Error deduplicating box scores: %v", err)
	}

	fmt.Println("Deduplication completed successfully!")
}

func deduplicateMatchups(year int) error {
	db := database.DB

	fmt.Printf("Deduplicating matchups for year %d...\n", year)

	// Count duplicates before
	var beforeCount int64
	db.Table("matchups").Where("year = ?", year).Count(&beforeCount)

	// Execute deduplication query
	result := db.Exec(`
		WITH unique_matchups AS (
			SELECT DISTINCT ON (home_team_id, away_team_id, week, year)
				id, created_at, updated_at, deleted_at, league_id, week, year, season,
				home_team_id, away_team_id, game_date, game_type, home_team_final_score,
				away_team_final_score, home_team_espn_projected_score, away_team_espn_projected_score,
				completed, is_playoff
			FROM matchups
			WHERE year = ?
			ORDER BY home_team_id, away_team_id, week, year, updated_at DESC
		)
		DELETE FROM matchups
		WHERE year = ?
		AND id NOT IN (SELECT id FROM unique_matchups)
	`, year, year)

	if result.Error != nil {
		return fmt.Errorf("failed to deduplicate matchups: %w", result.Error)
	}

	// Count after
	var afterCount int64
	db.Table("matchups").Where("year = ?", year).Count(&afterCount)

	fmt.Printf("Matchups for %d: %d -> %d (removed %d duplicates)\n",
		year, beforeCount, afterCount, beforeCount-afterCount)

	return nil
}

func deduplicateBoxScores(year int) error {
	db := database.DB

	fmt.Printf("Deduplicating box scores for year %d...\n", year)

	// Count duplicates before
	var beforeCount int64
	db.Table("box_scores").Where("year = ?", year).Count(&beforeCount)

	// Execute deduplication query
	result := db.Exec(`
		WITH unique_box_scores AS (
			SELECT DISTINCT ON (player_id, matchup_id, week, year)
				id, created_at, updated_at, deleted_at, matchup_id, player_id, team_id,
				week, year, slot_position, actual_points, projected_points, game_stats
			FROM box_scores bs
			WHERE year = ?
			  AND EXISTS (SELECT 1 FROM matchups m WHERE m.id = bs.matchup_id AND m.year = ?)
			ORDER BY player_id, matchup_id, week, year, updated_at DESC
		)
		DELETE FROM box_scores
		WHERE year = ?
		AND id NOT IN (SELECT id FROM unique_box_scores)
	`, year, year, year)

	if result.Error != nil {
		return fmt.Errorf("failed to deduplicate box scores: %w", result.Error)
	}

	// Count after
	var afterCount int64
	db.Table("box_scores").Where("year = ?", year).Count(&afterCount)

	fmt.Printf("Box scores for %d: %d -> %d (removed %d duplicates)\n",
		year, beforeCount, afterCount, beforeCount-afterCount)

	return nil
}

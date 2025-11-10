package etl

import (
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"encoding/json"
	"fmt"
	"os"

	"gorm.io/gorm"
)

type SimpleMatchup struct {
	Week           int    `json:"week"`
	Year           int    `json:"year"`
	GameType       string `json:"game_type"`
	IsPlayoff      bool   `json:"is_playoff"`
	HomeTeamESPNID int64  `json:"home_team_espn_id"`
	AwayTeamESPNID int64  `json:"away_team_espn_id"`
	Completed      bool   `json:"completed"`
}

func processPureMatchups(filePath string, createdTeams []*models.Team) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read pure matchups file %s: %w", filePath, err)
	}

	matchups := []SimpleMatchup{}
	if err := json.Unmarshal(data, &matchups); err != nil {
		return fmt.Errorf("failed to unmarshal pure matchups data from %s: %w", filePath, err)
	}

	for _, createdTeam := range createdTeams {
		logging.Infof("Created Team - ID: %d, ESPN ID: %d, Name: %s, Owner: %s",
			createdTeam.ID, createdTeam.ESPNID, createdTeam.Name, createdTeam.Owner)
	}

	for _, matchup := range matchups {
		logging.Infof("Pure Matchup - Week: %d, Year: %d, Home Team ESPN ID: %d, Away Team ESPN ID: %d, Completed: %t, Game Type: %s, Is Playoff: %t",
			matchup.Week, matchup.Year, matchup.HomeTeamESPNID, matchup.AwayTeamESPNID,
			matchup.Completed, matchup.GameType, matchup.IsPlayoff)

		entry := &models.Matchup{
			LeagueID:  leagueID,
			Week:      uint(matchup.Week),
			Year:      uint(matchup.Year),
			Season:    int(matchup.Year),
			Completed: matchup.Completed,
			GameType:  matchup.GameType,
			IsPlayoff: matchup.IsPlayoff,
		}
		// Look up team IDs from createdTeams
		for _, team := range createdTeams {
			if team.ESPNID == uint(matchup.HomeTeamESPNID) {
				entry.HomeTeamID = team.ID
			}
			if team.ESPNID == uint(matchup.AwayTeamESPNID) {
				entry.AwayTeamID = team.ID
			}
		}

		if entry.HomeTeamID == 0 {
			return fmt.Errorf("home team with ESPN ID %d not found in database", matchup.HomeTeamESPNID)
		}
		if entry.AwayTeamID == 0 {
			return fmt.Errorf("away team with ESPN ID %d not found in database", matchup.AwayTeamESPNID)
		}

		// Create or update the matchup in the database
		var existingMatchup models.Matchup
		err := database.DB.Where("home_team_id = ? AND away_team_id = ? AND week = ? AND year = ?",
			entry.HomeTeamID, entry.AwayTeamID, entry.Week, entry.Year).First(&existingMatchup).Error

		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return fmt.Errorf("error checking existing pure matchup for home team ID %d and away team ID %d: %w",
					entry.HomeTeamID, entry.AwayTeamID, err)
			}
			// Matchup does not exist, create a new one
			if createErr := database.DB.Create(entry).Error; createErr != nil {
				return fmt.Errorf("error creating new pure matchup for home team ID %d: %w", entry.HomeTeamID, createErr)
			}
			logging.Infof("Created new pure matchup: %s", entry)
		} else {
			// Matchup exists, update its details
			existingMatchup.Completed = entry.Completed
			existingMatchup.GameType = entry.GameType
			existingMatchup.IsPlayoff = entry.IsPlayoff

			if err := database.DB.Save(&existingMatchup).Error; err != nil {
				return fmt.Errorf("error updating existing pure matchup for home team ID %d: %w", entry.HomeTeamID, err)
			}
			logging.Infof("Updated existing pure matchup: %s", existingMatchup.String())
		}
	}

	logging.Infof("Successfully processed %d pure matchups from %s", len(matchups), filePath)
	return nil
}

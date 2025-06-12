package database

import "backend/internal/models"

// GetTeamsIDMap returns a map of all teams IDs (ESPN and Database) to the Team
func GetTeamsIDMap() (map[uint]models.Team, error) {
	var teams []models.Team
	if err := DB.Find(&teams).Error; err != nil {
		return nil, err
	}

	teamMap := make(map[uint]models.Team, len(teams))
	for _, team := range teams {
		teamMap[team.ESPNID] = team
		teamMap[team.ID] = team
	}
	return teamMap, nil

}

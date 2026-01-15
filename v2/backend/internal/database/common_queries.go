package database

import "backend/internal/models"

// TODO seankane: Wonder if this can be removed easily
// GetTeamsIDMap returns a map of all teams IDs (Platform-specific and Database) to the Team for a specific league
func GetTeamsIDMap(leagueID uint) (map[uint]models.Team, error) {
	var teams []models.Team
	if err := DB.Where("league_id = ?", leagueID).Find(&teams).Error; err != nil {
		return nil, err
	}

	teamMap := make(map[uint]models.Team, len(teams))
	for _, team := range teams {
		if team.TeamID != nil {
			teamMap[*team.TeamID] = team
		}
		teamMap[team.ID] = team
	}
	return teamMap, nil
}

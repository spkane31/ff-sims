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

// GetTeamsIDMapByLeague returns a map of team IDs (ESPN and Database) to Team, scoped to a league.
func GetTeamsIDMapByLeague(leagueID uint) (map[uint]models.Team, error) {
	var teams []models.Team
	if err := DB.Where("league_id = ?", leagueID).Find(&teams).Error; err != nil {
		return nil, err
	}

	teamMap := make(map[uint]models.Team, len(teams)*2)
	for _, team := range teams {
		teamMap[team.ESPNID] = team
		teamMap[team.ID] = team
	}
	return teamMap, nil
}

package simulation

import (
	"backend/internal/models"
	"errors"
	"math/rand"
	"time"
)

// ScheduleConfig holds configuration for schedule generation
type ScheduleConfig struct {
	NumTeams       int
	RegularWeeks   int
	PlayoffWeeks   int
	MaxGamesVsTeam int // Maximum games against same opponent (default: 2)
}

// ScheduleGenerator generates fantasy football schedules
type ScheduleGenerator struct {
	config ScheduleConfig
	rand   *rand.Rand
}

// NewScheduleGenerator creates a new schedule generator with the given config
func NewScheduleGenerator(config ScheduleConfig) *ScheduleGenerator {
	if config.MaxGamesVsTeam == 0 {
		config.MaxGamesVsTeam = 2
	}
	
	return &ScheduleGenerator{
		config: config,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateRegularSeasonSchedule creates a random schedule with constraints:
// 1. Each team can play another team at most MaxGamesVsTeam times
// 2. No team plays the same opponent in consecutive weeks
// 3. Each team plays exactly one game per week
func (sg *ScheduleGenerator) GenerateRegularSeasonSchedule(teams []models.Team, year uint, leagueID uint) ([]models.Matchup, error) {
	if len(teams)%2 != 0 {
		return nil, errors.New("number of teams must be even for schedule generation")
	}
	
	if len(teams) < 4 {
		return nil, errors.New("need at least 4 teams for schedule generation")
	}

	numTeams := len(teams)
	totalGamesNeeded := (numTeams * sg.config.RegularWeeks) / 2
	maxPossibleGames := (numTeams * (numTeams - 1) * sg.config.MaxGamesVsTeam) / 2
	
	if totalGamesNeeded > maxPossibleGames {
		return nil, errors.New("impossible to create schedule with given constraints")
	}

	var schedule []models.Matchup
	
	// Track how many times teams have played each other
	gamesPlayed := make(map[[2]uint]int)
	
	// Track last opponent for each team to prevent back-to-back games
	lastOpponent := make(map[uint]uint)
	
	for week := 1; week <= sg.config.RegularWeeks; week++ {
		weekMatches, err := sg.generateWeekMatches(teams, uint(week), year, leagueID, gamesPlayed, lastOpponent)
		if err != nil {
			// If we can't generate a week, try to backtrack
			if week > 1 {
				return nil, errors.New("failed to generate complete schedule - constraints too restrictive")
			}
			return nil, err
		}
		
		schedule = append(schedule, weekMatches...)
		
		// Update tracking maps
		for _, match := range weekMatches {
			key := sg.makeTeamPairKey(match.HomeTeamID, match.AwayTeamID)
			gamesPlayed[key]++
			lastOpponent[match.HomeTeamID] = match.AwayTeamID
			lastOpponent[match.AwayTeamID] = match.HomeTeamID
		}
	}
	
	return schedule, nil
}

// generateWeekMatches generates all matchups for a single week
func (sg *ScheduleGenerator) generateWeekMatches(teams []models.Team, week uint, year uint, leagueID uint, gamesPlayed map[[2]uint]int, lastOpponent map[uint]uint) ([]models.Matchup, error) {
	numTeams := len(teams)
	teamsAvailable := make([]uint, numTeams)
	for i, team := range teams {
		teamsAvailable[i] = team.ID
	}
	
	var matches []models.Matchup
	maxAttempts := 100
	
	for attempt := 0; attempt < maxAttempts; attempt++ {
		matches = []models.Matchup{}
		availableTeams := make([]uint, len(teamsAvailable))
		copy(availableTeams, teamsAvailable)
		
		// Shuffle teams for randomness
		sg.shuffleTeams(availableTeams)
		
		success := true
		
		// Try to pair up all teams
		for len(availableTeams) >= 2 {
			homeTeamID := availableTeams[0]
			availableTeams = availableTeams[1:]
			
			// Find a valid opponent for homeTeam
			validOpponentIndex := -1
			for i, awayTeamID := range availableTeams {
				if sg.canTeamsPlay(homeTeamID, awayTeamID, gamesPlayed, lastOpponent) {
					validOpponentIndex = i
					break
				}
			}
			
			if validOpponentIndex == -1 {
				// No valid opponent found, try different arrangement
				success = false
				break
			}
			
			awayTeamID := availableTeams[validOpponentIndex]
			availableTeams = append(availableTeams[:validOpponentIndex], availableTeams[validOpponentIndex+1:]...)
			
			// Create the matchup
			gameDate := time.Date(int(year), 9, int(week*7), 13, 0, 0, 0, time.UTC) // Sunday 1 PM
			match := models.Matchup{
				LeagueID:   leagueID,
				Week:       week,
				Year:       year,
				Season:     int(year),
				HomeTeamID: homeTeamID,
				AwayTeamID: awayTeamID,
				GameDate:   gameDate,
				GameType:   "regular",
				IsPlayoff:  false,
				Completed:  false,
			}
			
			matches = append(matches, match)
		}
		
		if success && len(availableTeams) == 0 {
			return matches, nil
		}
	}
	
	return nil, errors.New("unable to generate valid matchups for week after maximum attempts")
}

// canTeamsPlay checks if two teams can play against each other given constraints
func (sg *ScheduleGenerator) canTeamsPlay(team1ID, team2ID uint, gamesPlayed map[[2]uint]int, lastOpponent map[uint]uint) bool {
	// Check if teams already played maximum allowed games
	key := sg.makeTeamPairKey(team1ID, team2ID)
	if gamesPlayed[key] >= sg.config.MaxGamesVsTeam {
		return false
	}
	
	// Check if either team played the other team last week (no back-to-back)
	if lastOpponent[team1ID] == team2ID || lastOpponent[team2ID] == team1ID {
		return false
	}
	
	return true
}

// makeTeamPairKey creates a consistent key for team pairs (smaller ID first)
func (sg *ScheduleGenerator) makeTeamPairKey(team1ID, team2ID uint) [2]uint {
	if team1ID < team2ID {
		return [2]uint{team1ID, team2ID}
	}
	return [2]uint{team2ID, team1ID}
}

// shuffleTeams randomizes the order of team IDs
func (sg *ScheduleGenerator) shuffleTeams(teams []uint) {
	for i := range teams {
		j := sg.rand.Intn(i + 1)
		teams[i], teams[j] = teams[j], teams[i]
	}
}

// ValidateSchedule checks if a generated schedule meets all constraints
func (sg *ScheduleGenerator) ValidateSchedule(schedule []models.Matchup) error {
	if len(schedule) == 0 {
		return errors.New("empty schedule")
	}
	
	// Count games between teams
	gamesCount := make(map[[2]uint]int)
	
	// Track consecutive games for each team
	teamWeekOpponents := make(map[uint]map[uint]uint) // team -> week -> opponent
	
	for _, match := range schedule {
		// Count games between these teams
		key := sg.makeTeamPairKey(match.HomeTeamID, match.AwayTeamID)
		gamesCount[key]++
		
		// Track weekly opponents
		if teamWeekOpponents[match.HomeTeamID] == nil {
			teamWeekOpponents[match.HomeTeamID] = make(map[uint]uint)
		}
		if teamWeekOpponents[match.AwayTeamID] == nil {
			teamWeekOpponents[match.AwayTeamID] = make(map[uint]uint)
		}
		
		teamWeekOpponents[match.HomeTeamID][match.Week] = match.AwayTeamID
		teamWeekOpponents[match.AwayTeamID][match.Week] = match.HomeTeamID
	}
	
	// Validate max games constraint
	for _, count := range gamesCount {
		if count > sg.config.MaxGamesVsTeam {
			return errors.New("teams played more than maximum allowed games")
		}
	}
	
	// Validate no back-to-back games constraint
	for _, weekOpponents := range teamWeekOpponents {
		for week := uint(1); week < uint(sg.config.RegularWeeks); week++ {
			thisWeekOpponent, hasThisWeek := weekOpponents[week]
			nextWeekOpponent, hasNextWeek := weekOpponents[week+1]
			
			if hasThisWeek && hasNextWeek && thisWeekOpponent == nextWeekOpponent {
				return errors.New("team has back-to-back games against same opponent")
			}
		}
	}
	
	return nil
}

// GeneratePlayoffSchedule creates playoff matchups for top 6 teams
// Week 1 (Wildcard): 3v6, 4v5 (1st and 2nd get bye)
// Week 2 (Semifinals): 1v(lowest seed from week 1), 2v(highest seed from week 1) 
// Week 3 (Championship): Winners from week 2
func (sg *ScheduleGenerator) GeneratePlayoffSchedule(teams []models.Team, standings []TeamStanding, year uint, leagueID uint, startWeek uint) ([]models.Matchup, error) {
	if len(standings) < 6 {
		return nil, errors.New("need at least 6 teams for playoffs")
	}
	
	// Take top 6 teams from standings
	playoffTeams := standings[:6]
	
	var schedule []models.Matchup
	
	// Week 1 - Wildcard Round (seeds 3-6 play)
	wildcardWeek := startWeek
	wildcardMatches := []models.Matchup{
		sg.createPlayoffMatchup(playoffTeams[2].TeamID, playoffTeams[5].TeamID, wildcardWeek, year, leagueID, "wildcard"),
		sg.createPlayoffMatchup(playoffTeams[3].TeamID, playoffTeams[4].TeamID, wildcardWeek, year, leagueID, "wildcard"),
	}
	schedule = append(schedule, wildcardMatches...)
	
	// Week 2 - Semifinals
	semifinalWeek := startWeek + 1
	// Note: In actual implementation, winners would be determined from previous week
	// For schedule generation, we create placeholders
	semifinalMatches := []models.Matchup{
		sg.createPlayoffMatchup(playoffTeams[0].TeamID, 0, semifinalWeek, year, leagueID, "semifinal"), // 1 seed vs TBD
		sg.createPlayoffMatchup(playoffTeams[1].TeamID, 0, semifinalWeek, year, leagueID, "semifinal"), // 2 seed vs TBD
	}
	schedule = append(schedule, semifinalMatches...)
	
	// Week 3 - Championship
	championshipWeek := startWeek + 2
	championshipMatch := sg.createPlayoffMatchup(0, 0, championshipWeek, year, leagueID, "championship") // TBD vs TBD
	schedule = append(schedule, championshipMatch)
	
	return schedule, nil
}

// createPlayoffMatchup creates a single playoff matchup
func (sg *ScheduleGenerator) createPlayoffMatchup(homeTeamID, awayTeamID uint, week uint, year uint, leagueID uint, gameType string) models.Matchup {
	gameDate := time.Date(int(year), 12, int((week-14)*7), 13, 0, 0, 0, time.UTC) // December, Sunday 1 PM
	
	return models.Matchup{
		LeagueID:   leagueID,
		Week:       week,
		Year:       year,
		Season:     int(year),
		HomeTeamID: homeTeamID,
		AwayTeamID: awayTeamID,
		GameDate:   gameDate,
		GameType:   gameType,
		IsPlayoff:  true,
		Completed:  false,
	}
}

// TeamStanding represents a team's position in standings
type TeamStanding struct {
	TeamID      uint
	Wins        int
	Losses      int
	Ties        int
	Points      float64
	PlayoffSeed int
}
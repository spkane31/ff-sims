package simulation

import (
	"backend/internal/models"
	"fmt"
)

// SeasonSimulator generates complete valid season schedules for expected wins Monte Carlo simulation.
//
// WHY THIS EXISTS:
// The old approach randomly paired teams each week during simulations. This created unrealistic
// scenarios where a consistently low-scoring team could hypothetically play the highest-scoring
// team every single week, getting no credit for wins they'd earn against mid-tier teams.
//
// SOLUTION:
// Generate complete, valid season schedules that respect fantasy football constraints:
// - Each team plays exactly once per week
// - Teams play each opponent at most 2 times (1 time for round-robin)
// - No back-to-back games against the same opponent
//
// This gives a much more realistic expected wins calculation that properly accounts for
// schedule fairness and strength of schedule.
//
// USAGE IN ETL PIPELINE:
// 1. ETL loads completed matchups from YAML
// 2. processExpectedWinsForLeague() is called
// 3. For each of 10,000 simulations:
//    - SeasonSimulator generates a valid schedule
//    - Actual team scores are applied to the simulated schedule
//    - Wins are counted for each team
// 4. Results are averaged to get expected win probabilities
//
// EXAMPLE:
//   simulator := NewSeasonSimulator(teamIDs, 14, leagueID, year)
//   schedule, err := simulator.GenerateSchedule()
//   wins := applyScoresToSchedule(schedule, teamWeeklyScores, weeks)

// SimulatedSchedule represents a lightweight season schedule for Monte Carlo simulation
type SimulatedSchedule struct {
	Matchups []ScheduleMatchup
}

// ScheduleMatchup represents a single matchup in a simulated schedule
// This is a lightweight structure without full database model overhead
type ScheduleMatchup struct {
	Week       uint
	HomeTeamID uint
	AwayTeamID uint
}

// SeasonSimulator generates complete valid season schedules for Monte Carlo simulation
type SeasonSimulator struct {
	scheduleGenerator *ScheduleGenerator
	teamIDs           []uint
	numWeeks          int
	leagueID          uint
	year              uint
}

// NewSeasonSimulator creates a new season simulator for the given teams and configuration
func NewSeasonSimulator(teamIDs []uint, numWeeks int, leagueID, year uint) *SeasonSimulator {
	numTeams := len(teamIDs)

	// Calculate MaxGamesVsTeam based on number of teams and weeks
	// Use round-robin (max 1 game per opponent) when the math works out to exactly
	// one game per team pair: numWeeks ≈ numTeams - 1
	// Otherwise, allow up to 2 games per opponent
	maxGamesVsTeam := 2

	// Total games in schedule: (numTeams × numWeeks) / 2
	// Games in round-robin: (numTeams × (numTeams - 1)) / 2
	// If these are equal (or very close), it's round-robin
	totalGamesNeeded := (numTeams * numWeeks) / 2
	roundRobinGames := (numTeams * (numTeams - 1)) / 2

	if totalGamesNeeded == roundRobinGames || totalGamesNeeded == roundRobinGames+numTeams/2 {
		// Round-robin scenario: each team plays every other team exactly once
		maxGamesVsTeam = 1
	}

	// Create schedule generator config
	config := ScheduleConfig{
		NumTeams:       numTeams,
		RegularWeeks:   numWeeks,
		PlayoffWeeks:   0, // Not used for regular season simulation
		MaxGamesVsTeam: maxGamesVsTeam,
	}

	return &SeasonSimulator{
		scheduleGenerator: NewScheduleGenerator(config),
		teamIDs:           teamIDs,
		numWeeks:          numWeeks,
		leagueID:          leagueID,
		year:              year,
	}
}

// GenerateSchedule generates one complete valid season schedule
// Returns a lightweight SimulatedSchedule that can be used to apply scores
func (ss *SeasonSimulator) GenerateSchedule() (*SimulatedSchedule, error) {
	// Convert team IDs to temporary Team models for ScheduleGenerator
	teams := ss.createTeamModels()

	// Generate the schedule using existing ScheduleGenerator
	matchups, err := ss.scheduleGenerator.GenerateRegularSeasonSchedule(teams, ss.year, ss.leagueID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schedule: %w", err)
	}

	// Convert full Matchup models to lightweight ScheduleMatchup structs
	simSchedule := &SimulatedSchedule{
		Matchups: make([]ScheduleMatchup, len(matchups)),
	}

	for i, matchup := range matchups {
		simSchedule.Matchups[i] = ScheduleMatchup{
			Week:       matchup.Week,
			HomeTeamID: matchup.HomeTeamID,
			AwayTeamID: matchup.AwayTeamID,
		}
	}

	return simSchedule, nil
}

// createTeamModels converts team IDs to temporary Team models for ScheduleGenerator
func (ss *SeasonSimulator) createTeamModels() []models.Team {
	teams := make([]models.Team, len(ss.teamIDs))
	for i, teamID := range ss.teamIDs {
		teams[i] = models.Team{
			ID:       teamID,
			LeagueID: ss.leagueID,
			// Other fields not needed for schedule generation
		}
	}
	return teams
}

// ValidateSchedule validates that a generated schedule meets all constraints
// This is a convenience method that wraps the ScheduleGenerator's validation
func (ss *SeasonSimulator) ValidateSchedule(schedule *SimulatedSchedule) error {
	// Convert lightweight schedule back to full Matchup models for validation
	matchups := make([]models.Matchup, len(schedule.Matchups))
	for i, sm := range schedule.Matchups {
		matchups[i] = models.Matchup{
			Week:       sm.Week,
			Season:     ss.year,
			HomeTeamID: sm.HomeTeamID,
			AwayTeamID: sm.AwayTeamID,
			LeagueID:   ss.leagueID,
		}
	}

	return ss.scheduleGenerator.ValidateSchedule(matchups)
}

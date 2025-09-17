package handlers

import (
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/utils"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"sync"

	"github.com/gin-gonic/gin"
)

type GetSchedulesResponse struct {
	Data Schedule `json:"data"`
}

type Schedule struct {
	Matchups []Matchup `json:"matchups"`
}

type Matchup struct {
	ID                 string           `json:"id"`
	Year               uint             `json:"year"`
	Week               uint             `json:"week"`
	HomeTeamESPNID     uint             `json:"homeTeamESPNID"`
	AwayTeamESPNID     uint             `json:"awayTeamESPNID"`
	HomeTeamName       string           `json:"homeTeamName"`
	AwayTeamName       string           `json:"awayTeamName"`
	HomeScore          float64          `json:"homeScore"`
	AwayScore          float64          `json:"awayScore"`
	HomeProjectedScore float64          `json:"homeProjectedScore"`
	AwayProjectedScore float64          `json:"awayProjectedScore"`
	HomePlayers        []BoxScorePlayer `json:"homePlayers"`
	AwayPlayers        []BoxScorePlayer `json:"awayPlayers"`
	GameType           string           `json:"gameType"`
	PlayoffGameType    string           `json:"playoffGameType"`
}

// GetPlayers returns all players with optional filtering
func GetSchedules(c *gin.Context) {
	year := c.Query("year")
	gameTypeFilter := c.Query("gameType") // "all", "regular", "playoffs"

	slog.Info("Fetching schedules", "year", year, "gameType", gameTypeFilter)

	wg := sync.WaitGroup{}
	wg.Add(1)

	schedule, scheduleErr := []models.Matchup{}, error(nil)
	go func() {
		defer wg.Done()
		if year == "" {
			// Exclude games where both home and away scores are 0 (not truly completed games)
			if scheduleErr = database.DB.Model(&models.Matchup{}).Where("completed = true AND NOT (home_team_final_score = 0 AND away_team_final_score = 0)").Find(&schedule).Error; scheduleErr != nil {
				slog.Error("Failed to fetch schedules from database", "error", scheduleErr)
			}
		} else {
			// Exclude games where both home and away scores are 0 (not truly completed games)
			if scheduleErr = database.DB.Model(&models.Matchup{}).Where("year = ? AND completed = true AND NOT (home_team_final_score = 0 AND away_team_final_score = 0)", year).Find(&schedule).Error; scheduleErr != nil {
				slog.Error("Failed to fetch schedules from database", "error", scheduleErr)
			}
		}
		slog.Info("Fetched schedules from database", "count", len(schedule))
	}()

	teams, teamsErr := []models.Team{}, error(nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if teamsErr = database.DB.Model(&models.Team{}).Select("id, espn_id, owner").Find(&teams).Error; teamsErr != nil {
			slog.Error("Failed to fetch teams from database", "error", teamsErr)
			return
		}
		slog.Info("Fetched teams from database", "count", len(teams))
	}()

	wg.Wait()

	if teamsErr != nil {
		slog.Error("Error fetching teams", "error", teamsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch teams"})
		return
	}
	if scheduleErr != nil {
		slog.Error("Error fetching schedules", "error", scheduleErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch schedules"})
		return
	}

	// Apply server-side filtering using playoff detection logic
	filteredSchedule := utils.FilterPlayoffGames(schedule)

	// Further filter based on gameType query parameter
	if gameTypeFilter != "" && gameTypeFilter != "all" {
		var typeFilteredSchedule []models.Matchup
		for _, matchup := range filteredSchedule {
			playoffGameType := utils.GetPlayoffGameType(matchup, schedule)

			switch gameTypeFilter {
			case "regular":
				if playoffGameType == utils.PlayoffGameTypeRegular {
					typeFilteredSchedule = append(typeFilteredSchedule, matchup)
				}
			case "playoffs":
				if playoffGameType == utils.PlayoffGameTypePlayoff ||
					playoffGameType == utils.PlayoffGameTypeChampionship ||
					playoffGameType == utils.PlayoffGameTypeThirdPlace {
					typeFilteredSchedule = append(typeFilteredSchedule, matchup)
				}
			}
		}
		filteredSchedule = typeFilteredSchedule
	}

	resp := GetSchedulesResponse{}
	resp.Data.Matchups = make([]Matchup, len(filteredSchedule))
	for i, matchup := range filteredSchedule {
		playoffGameType := utils.GetPlayoffGameType(matchup, schedule)

		resp.Data.Matchups[i] = Matchup{
			ID:                 fmt.Sprintf("%d", matchup.ID),
			Year:               matchup.Year,
			Week:               matchup.Week,
			HomeScore:          matchup.HomeTeamFinalScore,
			AwayScore:          matchup.AwayTeamFinalScore,
			HomeProjectedScore: matchup.HomeTeamESPNProjectedScore,
			AwayProjectedScore: matchup.AwayTeamESPNProjectedScore,
			GameType:           matchup.GameType,
			PlayoffGameType:    string(playoffGameType),
		}

		for _, team := range teams {
			if team.ID == matchup.HomeTeamID {
				resp.Data.Matchups[i].HomeTeamName = team.Owner
				resp.Data.Matchups[i].HomeTeamESPNID = team.ESPNID
			}
			if team.ID == matchup.AwayTeamID {
				resp.Data.Matchups[i].AwayTeamName = team.Owner
				resp.Data.Matchups[i].AwayTeamESPNID = team.ESPNID
			}
		}
	}

	// Sort in reverse order by year and week
	slices.SortStableFunc(resp.Data.Matchups, func(a, b Matchup) int {
		if a.Year != b.Year {
			return int(b.Year - a.Year) // Reverse order by year
		}
		return int(b.Week - a.Week) // Reverse order by week
	})

	c.JSON(http.StatusOK, resp)
}

type GetMatchupResponse struct {
	Data SingleMatchup `json:"data"`
}

type SingleMatchup struct {
	ID             string      `json:"id"`
	Year           uint        `json:"year"`
	Week           uint        `json:"week"`
	HomeTeamESPNID uint        `json:"homeTeamESPNID"`
	AwayTeamESPNID uint        `json:"awayTeamESPNID"`
	HomeTeam       TeamMatchup `json:"homeTeam"`
	AwayTeam       TeamMatchup `json:"awayTeam"`
}

type TeamMatchup struct {
	ESPNID         string           `json:"id"`
	Name           string           `json:"name"`
	Score          float64          `json:"score"`
	ProjectedScore float64          `json:"projectedScore"`
	Players        []BoxScorePlayer `json:"players"`
}

type BoxScorePlayer struct {
	ID              string  `json:"id"`
	PlayerName      string  `json:"playerName"`
	PlayerPosition  string  `json:"playerPosition"`
	Status          string  `json:"status"`
	Team            string  `json:"team"`
	Points          float64 `json:"points"`
	ProjectedPoints float64 `json:"projectedPoints"`
	SlotPosition    string  `json:"slotPosition"`
	IsStarter       bool    `json:"isStarter"`
}

// getPositionOrder returns the sort order for fantasy positions
func getPositionOrder(position string) int {
	switch position {
	case "QB":
		return 1
	case "RB":
		return 2
	case "WR":
		return 3
	case "TE":
		return 4
	case "RB/WR/TE":
		return 5
	case "K":
		return 6
	case "D/ST", "DST":
		return 7
	case "BE":
		return 8
	case "IR":
		return 9
	default:
		return 10
	}
}

func GetMatchup(c *gin.Context) {
	id := c.Param("id")
	slog.Info("Fetching matchup", "id", id)

	wg := sync.WaitGroup{}
	wg.Add(3)

	matchups, matchupsErr := []models.Matchup{}, error(nil)
	go func() {
		defer wg.Done()
		if matchupsErr = database.DB.Model(&models.Matchup{}).Where("id = ?", id).Find(&matchups).Error; matchupsErr != nil {
			slog.Error("Failed to fetch matchup from database", "error", matchupsErr)
		}
	}()

	teams, teamsErr := []models.Team{}, error(nil)
	go func() {
		defer wg.Done()
		if teamsErr = database.DB.Model(&models.Team{}).Find(&teams).Error; teamsErr != nil {
			slog.Error("Failed to fetch teams from database", "error", teamsErr)
		}
	}()

	boxScores, boxScoresErr := []models.BoxScore{}, error(nil)
	go func() {
		defer wg.Done()
		if boxScoresErr = database.DB.Preload("Player").Where("matchup_id = ?", id).Find(&boxScores).Error; boxScoresErr != nil {
			slog.Error("Failed to fetch box scores from database", "error", boxScoresErr)
		}
	}()

	wg.Wait()

	if matchupsErr != nil {
		slog.Error("Error fetching matchup", "error", matchupsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch matchup"})
		return
	}

	if teamsErr != nil {
		slog.Error("Error fetching teams", "error", teamsErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch teams"})
		return
	}

	if boxScoresErr != nil {
		slog.Error("Error fetching box scores", "error", boxScoresErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch box scores"})
		return
	}

	if len(matchups) != 1 {
		slog.Error("Matchup not found", "id", id, "count", len(matchups))
		if len(matchups) == 0 {
			// No matchup found
			c.JSON(http.StatusNotFound, gin.H{"error": "Matchup not found"})
			return
		}
		// More than one matchup found, which is unexpected
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Multiple matchups found with the same ID"})
		return
	}

	matchup := matchups[0]

	// Find team names and ESPN IDs
	homeTeamName := "Unknown Team"
	awayTeamName := "Unknown Team"
	homeTeamESPNID := uint(0)
	awayTeamESPNID := uint(0)
	for _, team := range teams {
		if team.ID == matchup.HomeTeamID {
			homeTeamName = team.Owner
			homeTeamESPNID = team.ESPNID
		}
		if team.ID == matchup.AwayTeamID {
			awayTeamName = team.Owner
			awayTeamESPNID = team.ESPNID
		}
	}

	// Separate box scores by team and calculate projected scores
	var homeTeamPlayers []BoxScorePlayer
	var awayTeamPlayers []BoxScorePlayer
	var homeProjectedScore float64
	var awayProjectedScore float64

	for _, boxScore := range boxScores {
		player := BoxScorePlayer{
			ID:              fmt.Sprintf("%d", boxScore.PlayerID),
			PlayerName:      boxScore.Player.Name,
			PlayerPosition:  boxScore.Player.Position,
			Status:          boxScore.Player.Status,
			Team:            boxScore.Player.Team,
			Points:          boxScore.ActualPoints,
			ProjectedPoints: boxScore.ProjectedPoints,
			SlotPosition:    boxScore.SlotPosition,
			IsStarter:       boxScore.SlotPosition != "BE" && boxScore.SlotPosition != "IR" && boxScore.SlotPosition != "",
		}

		if boxScore.TeamID == matchup.HomeTeamID {
			homeTeamPlayers = append(homeTeamPlayers, player)
			// Only add projected points for starters (non-bench, non-IR players)
			if player.IsStarter {
				homeProjectedScore += boxScore.ProjectedPoints
			}
		} else if boxScore.TeamID == matchup.AwayTeamID {
			awayTeamPlayers = append(awayTeamPlayers, player)
			// Only add projected points for starters (non-bench, non-IR players)
			if player.IsStarter {
				awayProjectedScore += boxScore.ProjectedPoints
			}
		}
	}

	// Sort players by position order
	sort.Slice(homeTeamPlayers, func(i, j int) bool {
		return getPositionOrder(homeTeamPlayers[i].SlotPosition) < getPositionOrder(homeTeamPlayers[j].SlotPosition)
	})
	sort.Slice(awayTeamPlayers, func(i, j int) bool {
		return getPositionOrder(awayTeamPlayers[i].SlotPosition) < getPositionOrder(awayTeamPlayers[j].SlotPosition)
	})

	resp := GetMatchupResponse{
		Data: SingleMatchup{
			ID:             id,
			Year:           matchup.Year,
			Week:           matchup.Week,
			HomeTeamESPNID: homeTeamESPNID,
			AwayTeamESPNID: awayTeamESPNID,
			HomeTeam: TeamMatchup{
				ESPNID:         fmt.Sprintf("%d", homeTeamESPNID),
				Score:          matchup.HomeTeamFinalScore,
				ProjectedScore: homeProjectedScore,
				Name:           homeTeamName,
				Players:        homeTeamPlayers,
			},
			AwayTeam: TeamMatchup{
				ESPNID:         fmt.Sprintf("%d", awayTeamESPNID),
				Score:          matchup.AwayTeamFinalScore,
				ProjectedScore: awayProjectedScore,
				Name:           awayTeamName,
				Players:        awayTeamPlayers,
			},
		},
	}

	c.JSON(http.StatusOK, resp)
}

package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"backend/internal/database"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/exp/slices"
	"gorm.io/gorm"
)

type GetPlayersResponse struct {
	Players []PlayerSummaryResponse `json:"players"`
	Total   int64                   `json:"total"`
	Page    int                     `json:"page"`
	Limit   int                     `json:"limit"`
}

type PlayerSummaryResponse struct {
	ID                   string              `json:"id"`
	ESPNID               string              `json:"espnId"`
	Name                 string              `json:"name"`
	Position             string              `json:"position"`
	Team                 string              `json:"team"`
	Status               string              `json:"status"`
	TotalFantasyPoints   float64             `json:"totalFantasyPoints"`
	TotalProjectedPoints float64             `json:"totalProjectedPoints"`
	Difference           float64             `json:"difference"`
	GamesPlayed          int                 `json:"gamesPlayed"`
	AvgFantasyPoints     float64             `json:"avgFantasyPoints"`
	PositionRank         int                 `json:"positionRank"`
	TotalStats           PlayerStatsResponse `json:"totalStats"`
}

// GetPlayers returns all players with optional filtering and pagination
func GetPlayers(c *gin.Context) {
	position := c.Query("position")        // Filter by position if provided
	year := c.DefaultQuery("year", "2024") // Default to current year
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}

	offset := (page - 1) * limit

	slog.Info("Fetching players", "position", position, "year", year, "page", page, "limit", limit)

	var allPlayers []models.Player
	var totalCount int64
	var boxScores []models.BoxScore

	wg := sync.WaitGroup{}
	var playersErr, countErr, boxScoresErr error

	// Build query conditions
	query := database.DB.Model(&models.Player{})
	if position != "" {
		query = query.Where("position = ?", position)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		countErr = query.Count(&totalCount).Error
		if countErr != nil {
			slog.Error("Failed to count players", "error", countErr)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		playersQuery := database.DB.Model(&models.Player{}).Offset(offset).Limit(limit).Order("name ASC")
		playersErr = playersQuery.Find(&allPlayers).Error
		if playersErr != nil {
			slog.Error("Failed to fetch players from database", "error", playersErr)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		yearInt, _ := strconv.Atoi(year)
		boxScoresErr = database.DB.Where("year = ?", yearInt).Find(&boxScores).Error
		if boxScoresErr != nil {
			slog.Error("Failed to fetch box scores from database", "error", boxScoresErr)
		}
	}()

	wg.Wait()

	if playersErr != nil {
		slog.Error("Error fetching players", "error", playersErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch players"})
		return
	}
	if countErr != nil {
		slog.Error("Error counting players", "error", countErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count players"})
		return
	}
	if boxScoresErr != nil {
		slog.Error("Error fetching box scores", "error", boxScoresErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch box scores"})
		return
	}

	slog.Info("Fetched data from database", "players", len(allPlayers), "total_count", totalCount, "box_scores", len(boxScores))

	// Create map for quick box score lookups by player ID
	playerBoxScores := make(map[uint][]models.BoxScore)
	for _, boxScore := range boxScores {
		playerBoxScores[boxScore.PlayerID] = append(playerBoxScores[boxScore.PlayerID], boxScore)
	}

	resp := GetPlayersResponse{
		Total: totalCount,
		Page:  page,
		Limit: limit,
	}

	// Process each player and aggregate their statistics
	for _, player := range allPlayers {
		playerScores := playerBoxScores[player.ID]

		var totalFantasyPoints, totalProjectedPoints float64
		var totalStats PlayerStatsResponse
		gamesPlayed := len(playerScores)

		for _, boxScore := range playerScores {
			totalFantasyPoints += boxScore.ActualPoints
			totalProjectedPoints += boxScore.ProjectedPoints

			// Aggregate stats
			totalStats.PassingYards += int(boxScore.GameStats.PassingYards)
			totalStats.PassingTDs += int(boxScore.GameStats.PassingTDs)
			totalStats.Interceptions += int(boxScore.GameStats.Interceptions)
			totalStats.RushingYards += int(boxScore.GameStats.RushingYards)
			totalStats.RushingTDs += int(boxScore.GameStats.RushingTDs)
			totalStats.Receptions += int(boxScore.GameStats.Receptions)
			totalStats.ReceivingYards += int(boxScore.GameStats.ReceivingYards)
			totalStats.ReceivingTDs += int(boxScore.GameStats.ReceivingTDs)
			totalStats.Fumbles += int(boxScore.GameStats.Fumbles)
			totalStats.FieldGoals += int(boxScore.GameStats.FieldGoals)
			totalStats.ExtraPoints += int(boxScore.GameStats.ExtraPoints)
		}

		avgFantasyPoints := 0.0
		if gamesPlayed > 0 {
			avgFantasyPoints = totalFantasyPoints / float64(gamesPlayed)
		}

		difference := totalFantasyPoints - totalProjectedPoints

		resp.Players = append(resp.Players, PlayerSummaryResponse{
			ID:                   strconv.FormatUint(uint64(player.ID), 10),
			ESPNID:               strconv.FormatInt(player.ESPNID, 10),
			Name:                 player.Name,
			Position:             player.Position,
			Team:                 player.Team,
			Status:               player.Status,
			TotalFantasyPoints:   totalFantasyPoints,
			TotalProjectedPoints: totalProjectedPoints,
			Difference:           difference,
			GamesPlayed:          gamesPlayed,
			AvgFantasyPoints:     avgFantasyPoints,
			TotalStats:           totalStats,
		})
	}

	// Sort players by total fantasy points (descending)
	slices.SortStableFunc(resp.Players, func(a, b PlayerSummaryResponse) int {
		if a.TotalFantasyPoints != b.TotalFantasyPoints {
			if a.TotalFantasyPoints < b.TotalFantasyPoints {
				return 1
			}
			return -1
		}
		return 0
	})

	// Calculate position ranks
	positionRanks := make(map[string]int)
	for i := range resp.Players {
		position := resp.Players[i].Position
		positionRanks[position]++
		resp.Players[i].PositionRank = positionRanks[position]
	}

	c.JSON(http.StatusOK, resp)
}

// GetPlayerByID returns a player by ID with detailed statistics
func GetPlayerByID(c *gin.Context) {
	id := c.Param("id")

	slog.Info("Fetching player by ID", "id", id)

	// Convert ID to uint
	playerID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		slog.Error("Invalid player ID", "id", id, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid player ID"})
		return
	}

	// Fetch player from database
	var player models.Player
	if err := database.DB.Where("id = ?", playerID).First(&player).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			slog.Warn("Player not found", "id", id)
			c.JSON(http.StatusNotFound, gin.H{"error": "Player not found"})
			return
		}
		slog.Error("Failed to fetch player from database", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch player"})
		return
	}

	// Fetch box scores for the player (default to current year)
	year := c.DefaultQuery("year", "2024")
	yearInt, _ := strconv.Atoi(year)

	var boxScores []models.BoxScore
	if err := database.DB.Where("player_id = ? AND year = ?", player.ID, yearInt).
		Order("week asc").Find(&boxScores).Error; err != nil {
		slog.Error("Failed to fetch box scores", "error", err, "player_id", player.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch player statistics"})
		return
	}

	// Aggregate statistics
	var totalFantasyPoints, totalProjectedPoints float64
	var totalStats PlayerStatsResponse
	gamesPlayed := len(boxScores)

	for _, boxScore := range boxScores {
		totalFantasyPoints += boxScore.ActualPoints
		totalProjectedPoints += boxScore.ProjectedPoints

		// Aggregate stats
		totalStats.PassingYards += int(boxScore.GameStats.PassingYards)
		totalStats.PassingTDs += int(boxScore.GameStats.PassingTDs)
		totalStats.Interceptions += int(boxScore.GameStats.Interceptions)
		totalStats.RushingYards += int(boxScore.GameStats.RushingYards)
		totalStats.RushingTDs += int(boxScore.GameStats.RushingTDs)
		totalStats.Receptions += int(boxScore.GameStats.Receptions)
		totalStats.ReceivingYards += int(boxScore.GameStats.ReceivingYards)
		totalStats.ReceivingTDs += int(boxScore.GameStats.ReceivingTDs)
		totalStats.Fumbles += int(boxScore.GameStats.Fumbles)
		totalStats.FieldGoals += int(boxScore.GameStats.FieldGoals)
		totalStats.ExtraPoints += int(boxScore.GameStats.ExtraPoints)
	}

	avgFantasyPoints := 0.0
	if gamesPlayed > 0 {
		avgFantasyPoints = totalFantasyPoints / float64(gamesPlayed)
	}

	difference := totalFantasyPoints - totalProjectedPoints

	// Calculate position rank by fetching all players of same position and comparing total fantasy points
	var playersInPosition []models.Player
	if err := database.DB.Where("position = ?", player.Position).Find(&playersInPosition).Error; err != nil {
		slog.Error("Failed to fetch players for position ranking", "error", err, "position", player.Position)
	}

	positionRank := 1
	for _, otherPlayer := range playersInPosition {
		if otherPlayer.ID == player.ID {
			continue
		}

		// Get other player's box scores for comparison
		var otherBoxScores []models.BoxScore
		if err := database.DB.Where("player_id = ? AND year = ?", otherPlayer.ID, yearInt).Find(&otherBoxScores).Error; err != nil {
			continue
		}

		var otherTotalPoints float64
		for _, bs := range otherBoxScores {
			otherTotalPoints += bs.ActualPoints
		}

		if otherTotalPoints > totalFantasyPoints {
			positionRank++
		}
	}

	// Create response
	response := gin.H{
		"id":                   strconv.FormatUint(uint64(player.ID), 10),
		"espnId":               strconv.FormatInt(player.ESPNID, 10),
		"name":                 player.Name,
		"position":             player.Position,
		"team":                 player.Team,
		"status":               player.Status,
		"totalFantasyPoints":   totalFantasyPoints,
		"totalProjectedPoints": totalProjectedPoints,
		"difference":           difference,
		"gamesPlayed":          gamesPlayed,
		"avgFantasyPoints":     avgFantasyPoints,
		"positionRank":         positionRank,
		"totalStats":           totalStats,
	}

	c.JSON(http.StatusOK, response)
}

// GetPlayerStats returns player statistics
func GetPlayerStats(c *gin.Context) {
	// In a real implementation, you would query the database
	// Optionally filter by week, season, etc.
	week := c.DefaultQuery("week", "all")
	season := c.DefaultQuery("season", "2023")

	stats := []models.PlayerGameStats{
		{
			PlayerID:   1,
			PlayerName: "Tom Brady",
			Week:       1,
			Season:     2023,
			GameStats: models.PlayerStats{
				PassingYards:   312,
				PassingTDs:     3,
				RushingYards:   5,
				RushingTDs:     0,
				Receptions:     0,
				ReceivingYards: 0,
				ReceivingTDs:   0,
			},
			FantasyPoints: 25.7,
		},
		// Add more mock data
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
		"filters": gin.H{
			"week":   week,
			"season": season,
		},
	})
}

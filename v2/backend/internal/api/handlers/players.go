package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"backend/internal/database"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PlayerStatsResponse struct {
	PassingYards   int `json:"passingYards"`
	PassingTDs     int `json:"passingTDs"`
	Interceptions  int `json:"interceptions"`
	RushingYards   int `json:"rushingYards"`
	RushingTDs     int `json:"rushingTDs"`
	Receptions     int `json:"receptions"`
	ReceivingYards int `json:"receivingYards"`
	ReceivingTDs   int `json:"receivingTDs"`
	Fumbles        int `json:"fumbles"`
	FieldGoals     int `json:"fieldGoals"`
	ExtraPoints    int `json:"extraPoints"`
}

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
		limit = 50
	}

	offset := (page - 1) * limit

	slog.Info("Fetching players", "position", position, "year", year, "page", page, "limit", limit)

	// Build the SQL query similar to your provided query
	// SELECT p.id, p.name, p.position, p.team, p.status, p.espn_id,
	//        SUM(bs.actual_points) AS total_points,
	//        SUM(bs.projected_points) AS total_proj_points,
	//        COUNT(bs.id) AS games_played
	// FROM players p
	// JOIN box_scores bs ON p.id = bs.player_id
	// WHERE bs.year = ? [AND p.position = ?]
	// GROUP BY p.id, p.name, p.position, p.team, p.status, p.espn_id
	// ORDER BY total_points DESC
	// LIMIT ? OFFSET ?

	type PlayerAggregateResult struct {
		ID                   uint    `json:"id"`
		Name                 string  `json:"name"`
		Position             string  `json:"position"`
		Team                 string  `json:"team"`
		Status               string  `json:"status"`
		ESPNID               int64   `json:"espn_id"`
		TotalPoints          float64 `json:"total_points"`
		TotalProjectedPoints float64 `json:"total_projected_points"`
		GamesPlayed          int     `json:"games_played"`
	}

	var results []PlayerAggregateResult
	var totalCount int64

	// Build the query with joins
	query := database.DB.Table("players p").
		Select(`p.id, p.name, p.position, p.team, p.status, p.espn_id,
			COALESCE(SUM(bs.actual_points), 0) AS total_points,
			COALESCE(SUM(bs.projected_points), 0) AS total_projected_points,
			COALESCE(COUNT(bs.id), 0) AS games_played`).
		Joins("LEFT JOIN box_scores bs ON p.id = bs.player_id AND bs.year = ?", year).
		Group("p.id, p.name, p.position, p.team, p.status, p.espn_id")

	// Add position filter if provided
	if position != "" {
		query = query.Where("p.position = ?", position)
	}

	// Get total count for pagination
	countQuery := database.DB.Table("players p")
	if position != "" {
		countQuery = countQuery.Where("position = ?", position)
	}
	if err := countQuery.Count(&totalCount).Error; err != nil {
		slog.Error("Failed to count players", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count players"})
		return
	}

	// Execute the main query with pagination and ordering
	if err := query.Order("total_points DESC").
		Limit(limit).
		Offset(offset).
		Find(&results).Error; err != nil {
		slog.Error("Failed to fetch player aggregates", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch players"})
		return
	}

	slog.Info("Fetched player aggregates", "count", len(results), "total_count", totalCount)

	// For detailed stats, we still need to fetch box scores for these specific players
	playerIDs := make([]uint, len(results))
	for i, result := range results {
		playerIDs[i] = result.ID
	}

	var boxScores []models.BoxScore
	if len(playerIDs) > 0 {
		yearInt, _ := strconv.Atoi(year)
		if err := database.DB.Where("player_id IN ? AND year = ?", playerIDs, yearInt).Find(&boxScores).Error; err != nil {
			slog.Error("Failed to fetch detailed box scores", "error", err)
			// Continue without detailed stats rather than failing completely
		}
	}

	// Create map for detailed stats lookup
	playerDetailedStats := make(map[uint]PlayerStatsResponse)
	for _, boxScore := range boxScores {
		stats := playerDetailedStats[boxScore.PlayerID]
		stats.PassingYards += int(boxScore.GameStats.PassingYards)
		stats.PassingTDs += int(boxScore.GameStats.PassingTDs)
		stats.Interceptions += int(boxScore.GameStats.Interceptions)
		stats.RushingYards += int(boxScore.GameStats.RushingYards)
		stats.RushingTDs += int(boxScore.GameStats.RushingTDs)
		stats.Receptions += int(boxScore.GameStats.Receptions)
		stats.ReceivingYards += int(boxScore.GameStats.ReceivingYards)
		stats.ReceivingTDs += int(boxScore.GameStats.ReceivingTDs)
		stats.Fumbles += int(boxScore.GameStats.Fumbles)
		stats.FieldGoals += int(boxScore.GameStats.FieldGoals)
		stats.ExtraPoints += int(boxScore.GameStats.ExtraPoints)
		playerDetailedStats[boxScore.PlayerID] = stats
	}

	// Calculate position ranks within the current result set
	positionRanks := make(map[string]int)

	resp := GetPlayersResponse{
		Total: totalCount,
		Page:  page,
		Limit: limit,
	}

	// Convert results to response format
	for _, result := range results {
		avgFantasyPoints := 0.0
		if result.GamesPlayed > 0 {
			avgFantasyPoints = result.TotalPoints / float64(result.GamesPlayed)
		}

		difference := result.TotalPoints - result.TotalProjectedPoints

		// Calculate position rank
		positionRanks[result.Position]++

		resp.Players = append(resp.Players, PlayerSummaryResponse{
			ID:                   strconv.FormatUint(uint64(result.ID), 10),
			ESPNID:               strconv.FormatInt(result.ESPNID, 10),
			Name:                 result.Name,
			Position:             result.Position,
			Team:                 result.Team,
			Status:               result.Status,
			TotalFantasyPoints:   result.TotalPoints,
			TotalProjectedPoints: result.TotalProjectedPoints,
			Difference:           difference,
			GamesPlayed:          result.GamesPlayed,
			AvgFantasyPoints:     avgFantasyPoints,
			PositionRank:         positionRanks[result.Position],
			TotalStats:           playerDetailedStats[result.ID],
		})
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

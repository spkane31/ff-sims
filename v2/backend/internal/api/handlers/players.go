package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"backend/internal/database"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"math"
)

// calculateStandardDeviation calculates the standard deviation of a slice of float64 values
func calculateStandardDeviation(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate variance
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))

	// Return standard deviation
	return math.Sqrt(variance)
}

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

type GameLogEntry struct {
	Week            uint                `json:"week"`
	Year            uint                `json:"year"`
	ActualPoints    float64             `json:"actualPoints"`
	ProjectedPoints float64             `json:"projectedPoints"`
	Difference      float64             `json:"difference"`
	StartedFlag     bool                `json:"startedFlag"`
	GameDate        string              `json:"gameDate"`
	Stats           PlayerStatsResponse `json:"stats"`
}

type GamePerformance struct {
	Points float64 `json:"points"`
	Year   uint    `json:"year"`
	Week   uint    `json:"week"`
}

type AnnualStatsEntry struct {
	Year                 uint                `json:"year"`
	GamesPlayed          int                 `json:"gamesPlayed"`
	TotalFantasyPoints   float64             `json:"totalFantasyPoints"`
	TotalProjectedPoints float64             `json:"totalProjectedPoints"`
	AvgFantasyPoints     float64             `json:"avgFantasyPoints"`
	Difference           float64             `json:"difference"`
	BestGame             GamePerformance     `json:"bestGame"`
	WorstGame            GamePerformance     `json:"worstGame"`
	ConsistencyScore     float64             `json:"consistencyScore"` // Standard deviation
	TotalStats           PlayerStatsResponse `json:"totalStats"`
}

type PlayerDetailResponse struct {
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
	BestGame             GamePerformance     `json:"bestGame"`
	WorstGame            GamePerformance     `json:"worstGame"`
	ConsistencyScore     float64             `json:"consistencyScore"` // Standard deviation
	TotalStats           PlayerStatsResponse `json:"totalStats"`
	AnnualStats          []AnnualStatsEntry  `json:"annualStats"`
	GameLog              []GameLogEntry      `json:"gameLog"`
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
	position := c.Query("position")                  // Filter by position if provided
	year := c.DefaultQuery("year", "all")            // Default to all years for career stats
	rank := c.DefaultQuery("rank", "fantasy_points") // Ranking method: fantasy_points, avg_points, projected_points, games_played, vs_projection
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}

	offset := (page - 1) * limit

	slog.Info("Fetching players", "position", position, "year", year, "rank", rank, "page", page, "limit", limit)

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
			COALESCE(COUNT(bs.id), 0) AS games_played`)

	// Add year filter if not "all"
	if year == "all" {
		query = query.Joins("LEFT JOIN box_scores bs ON p.id = bs.player_id")
	} else {
		yearInt, _ := strconv.Atoi(year)
		query = query.Joins("LEFT JOIN box_scores bs ON p.id = bs.player_id AND bs.year = ?", yearInt)
	}

	query = query.Group("p.id, p.name, p.position, p.team, p.status, p.espn_id")

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

	// Determine ordering based on rank parameter
	var orderBy string
	switch rank {
	case "fantasy_points":
		orderBy = "COALESCE(SUM(bs.actual_points), 0) DESC"
	case "avg_points":
		orderBy = "CASE WHEN COALESCE(COUNT(bs.id), 0) > 0 THEN COALESCE(SUM(bs.actual_points), 0) / COALESCE(COUNT(bs.id), 0) ELSE 0 END DESC"
	case "projected_points":
		orderBy = "COALESCE(SUM(bs.projected_points), 0) DESC"
	case "games_played":
		orderBy = "COALESCE(COUNT(bs.id), 0) DESC"
	case "vs_projection":
		orderBy = "(COALESCE(SUM(bs.actual_points), 0) - COALESCE(SUM(bs.projected_points), 0)) DESC"
	default:
		orderBy = "COALESCE(SUM(bs.actual_points), 0) DESC" // Default to fantasy points
	}

	// Execute the main query with pagination and ordering
	if err := query.Order(orderBy).
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
		if year == "all" {
			if err := database.DB.Where("player_id IN ?", playerIDs).Find(&boxScores).Error; err != nil {
				slog.Error("Failed to fetch detailed box scores", "error", err)
				// Continue without detailed stats rather than failing completely
			}
		} else {
			yearInt, _ := strconv.Atoi(year)
			if err := database.DB.Where("player_id IN ? AND year = ?", playerIDs, yearInt).Find(&boxScores).Error; err != nil {
				slog.Error("Failed to fetch detailed box scores", "error", err)
				// Continue without detailed stats rather than failing completely
			}
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

	// Fetch all box scores for the player
	year := c.DefaultQuery("year", "all")

	var boxScores []models.BoxScore
	if year == "all" {
		if err := database.DB.Where("player_id = ?", player.ID).
			Order("year desc, week asc").Find(&boxScores).Error; err != nil {
			slog.Error("Failed to fetch box scores", "error", err, "player_id", player.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch player statistics"})
			return
		}
	} else {
		yearInt, _ := strconv.Atoi(year)
		if err := database.DB.Where("player_id = ? AND year = ?", player.ID, yearInt).
			Order("week asc").Find(&boxScores).Error; err != nil {
			slog.Error("Failed to fetch box scores", "error", err, "player_id", player.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch player statistics"})
			return
		}
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

	// TODO: Position rank calculation removed for performance
	// Will be implemented using pre-calculated rankings in player_season_stats table
	positionRank := 0

	// Calculate annual statistics
	yearlyStats := make(map[uint]*AnnualStatsEntry)
	yearlyGamePoints := make(map[uint][]float64) // Track individual game scores for standard deviation

	for _, boxScore := range boxScores {
		year := boxScore.Year
		if yearlyStats[year] == nil {
			yearlyStats[year] = &AnnualStatsEntry{
				Year:                 year,
				GamesPlayed:          0,
				TotalFantasyPoints:   0,
				TotalProjectedPoints: 0,
				AvgFantasyPoints:     0,
				Difference:           0,
				BestGame:             GamePerformance{Points: 0, Year: year, Week: 0},
				WorstGame:            GamePerformance{Points: math.MaxFloat64, Year: year, Week: 0},
				ConsistencyScore:     0,
				TotalStats:           PlayerStatsResponse{},
			}
			yearlyGamePoints[year] = []float64{}
		}

		entry := yearlyStats[year]
		entry.GamesPlayed++
		entry.TotalFantasyPoints += boxScore.ActualPoints
		entry.TotalProjectedPoints += boxScore.ProjectedPoints

		// Track individual game points
		yearlyGamePoints[year] = append(yearlyGamePoints[year], boxScore.ActualPoints)

		// Update best game
		if boxScore.ActualPoints > entry.BestGame.Points {
			entry.BestGame = GamePerformance{
				Points: boxScore.ActualPoints,
				Year:   boxScore.Year,
				Week:   boxScore.Week,
			}
		}

		// Update worst game (exclude 0-point games which are likely bye weeks)
		if boxScore.ActualPoints > 0 && boxScore.ActualPoints < entry.WorstGame.Points {
			entry.WorstGame = GamePerformance{
				Points: boxScore.ActualPoints,
				Year:   boxScore.Year,
				Week:   boxScore.Week,
			}
		}

		// Aggregate stats for the year
		entry.TotalStats.PassingYards += int(boxScore.GameStats.PassingYards)
		entry.TotalStats.PassingTDs += int(boxScore.GameStats.PassingTDs)
		entry.TotalStats.Interceptions += int(boxScore.GameStats.Interceptions)
		entry.TotalStats.RushingYards += int(boxScore.GameStats.RushingYards)
		entry.TotalStats.RushingTDs += int(boxScore.GameStats.RushingTDs)
		entry.TotalStats.Receptions += int(boxScore.GameStats.Receptions)
		entry.TotalStats.ReceivingYards += int(boxScore.GameStats.ReceivingYards)
		entry.TotalStats.ReceivingTDs += int(boxScore.GameStats.ReceivingTDs)
		entry.TotalStats.Fumbles += int(boxScore.GameStats.Fumbles)
		entry.TotalStats.FieldGoals += int(boxScore.GameStats.FieldGoals)
		entry.TotalStats.ExtraPoints += int(boxScore.GameStats.ExtraPoints)
	}

	// Calculate averages, differences, and consistency scores for each year
	var annualStats []AnnualStatsEntry
	for year, entry := range yearlyStats {
		if entry.GamesPlayed > 0 {
			entry.AvgFantasyPoints = entry.TotalFantasyPoints / float64(entry.GamesPlayed)
		}
		entry.Difference = entry.TotalFantasyPoints - entry.TotalProjectedPoints

		// Calculate consistency score (standard deviation)
		if gamePoints, exists := yearlyGamePoints[year]; exists {
			entry.ConsistencyScore = calculateStandardDeviation(gamePoints)
		}

		// Reset worst game if it was never set (no games played)
		if entry.WorstGame.Points == math.MaxFloat64 {
			entry.WorstGame.Points = 0
		}

		// Only include years where player actually played (has non-zero stats)
		hasStats := entry.TotalStats.PassingYards > 0 || entry.TotalStats.RushingYards > 0 ||
			entry.TotalStats.Receptions > 0 || entry.TotalStats.FieldGoals > 0 ||
			entry.TotalStats.ExtraPoints > 0 || entry.TotalFantasyPoints > 0

		if hasStats {
			annualStats = append(annualStats, *entry)
		}
	}

	// Sort annual stats by year (descending)
	for i := 0; i < len(annualStats)-1; i++ {
		for j := i + 1; j < len(annualStats); j++ {
			if annualStats[i].Year < annualStats[j].Year {
				annualStats[i], annualStats[j] = annualStats[j], annualStats[i]
			}
		}
	}

	// Calculate overall best game, worst game, and consistency
	var overallBestGame GamePerformance
	var overallWorstGame GamePerformance = GamePerformance{Points: math.MaxFloat64}
	var allGamePoints []float64

	for _, boxScore := range boxScores {
		allGamePoints = append(allGamePoints, boxScore.ActualPoints)

		if boxScore.ActualPoints > overallBestGame.Points {
			overallBestGame = GamePerformance{
				Points: boxScore.ActualPoints,
				Year:   boxScore.Year,
				Week:   boxScore.Week,
			}
		}

		if boxScore.ActualPoints > 0 && boxScore.ActualPoints < overallWorstGame.Points {
			overallWorstGame = GamePerformance{
				Points: boxScore.ActualPoints,
				Year:   boxScore.Year,
				Week:   boxScore.Week,
			}
		}
	}

	// Reset worst game if it was never set
	if overallWorstGame.Points == math.MaxFloat64 {
		overallWorstGame.Points = 0
	}

	overallConsistencyScore := calculateStandardDeviation(allGamePoints)

	// Build game log entries from box scores
	var gameLog []GameLogEntry
	for _, boxScore := range boxScores {
		gameStats := PlayerStatsResponse{
			PassingYards:   int(boxScore.GameStats.PassingYards),
			PassingTDs:     int(boxScore.GameStats.PassingTDs),
			Interceptions:  int(boxScore.GameStats.Interceptions),
			RushingYards:   int(boxScore.GameStats.RushingYards),
			RushingTDs:     int(boxScore.GameStats.RushingTDs),
			Receptions:     int(boxScore.GameStats.Receptions),
			ReceivingYards: int(boxScore.GameStats.ReceivingYards),
			ReceivingTDs:   int(boxScore.GameStats.ReceivingTDs),
			Fumbles:        int(boxScore.GameStats.Fumbles),
			FieldGoals:     int(boxScore.GameStats.FieldGoals),
			ExtraPoints:    int(boxScore.GameStats.ExtraPoints),
		}

		gameLogEntry := GameLogEntry{
			Week:            boxScore.Week,
			Year:            boxScore.Year,
			ActualPoints:    boxScore.ActualPoints,
			ProjectedPoints: boxScore.ProjectedPoints,
			Difference:      boxScore.ActualPoints - boxScore.ProjectedPoints,
			StartedFlag:     boxScore.StartedFlag,
			GameDate:        boxScore.GameDate.Format("2006-01-02"),
			Stats:           gameStats,
		}

		gameLog = append(gameLog, gameLogEntry)
	}

	// Create response
	response := PlayerDetailResponse{
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
		PositionRank:         positionRank,
		BestGame:             overallBestGame,
		WorstGame:            overallWorstGame,
		ConsistencyScore:     overallConsistencyScore,
		TotalStats:           totalStats,
		AnnualStats:          annualStats,
		GameLog:              gameLog,
	}

	c.JSON(http.StatusOK, response)
}

// GetPlayerStats returns player statistics
func GetPlayerStats(c *gin.Context) {
	// In a real implementation, you would query the database
	// Optionally filter by week, season, etc.
	week := c.DefaultQuery("week", "all")
	season := c.DefaultQuery("season", "2023")

	c.JSON(http.StatusOK, gin.H{
		"stats": map[string]int{},
		"filters": gin.H{
			"week":   week,
			"season": season,
		},
	})
}

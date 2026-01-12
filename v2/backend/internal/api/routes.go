package api

import (
	"backend/internal/api/handlers"
	"backend/internal/api/middleware"

	"github.com/gin-gonic/gin"
)

// SetupRouter configures the API routes
func SetupRouter(r *gin.Engine) {
	// Health check endpoint (no league context needed)
	r.GET("/api/health", handlers.HealthCheck)

	// Global leagues endpoint (lists all leagues)
	r.GET("/api/leagues", handlers.GetLeagues)

	// TODO (seankane): Remove the legacy routes on push
	// // Legacy API routes with default league middleware (backward compatible)
	// legacyAPI := r.Group("/api")
	// legacyAPI.Use(middleware.DefaultLeagueMiddleware(345674))
	// {
	// 	setupLeagueRoutes(legacyAPI)
	// }

	// New multi-league routes with league context from URL
	leagueAPI := r.Group("/api/league/:leagueId")
	leagueAPI.Use(middleware.LeagueContextMiddleware())
	{
		setupLeagueRoutes(leagueAPI)
	}
}

// setupLeagueRoutes configures routes that are league-scoped
// Used by both legacy (/api/*) and new multi-league (/api/league/:leagueId/*) routes
func setupLeagueRoutes(group *gin.RouterGroup) {
	// Teams endpoints
	teams := group.Group("/teams")
	{
		teams.GET("", handlers.GetTeams)
		teams.GET("/:id", handlers.GetTeamByID)
		teams.GET("/all-time-expected-wins", handlers.GetAllTimeExpectedWins)
		teams.GET("/standings/:year", handlers.GetCurrentSeasonStandings)
		teams.GET("/:id/expected-wins/:year", handlers.GetTeamProgression)
	}

	// Players endpoints (global data, but can be filtered by league context)
	players := group.Group("/players")
	{
		players.GET("", handlers.GetPlayers)
		players.GET("/:id", handlers.GetPlayerByID)
		players.GET("/stats", handlers.GetPlayerStats)
	}

	// Schedules endpoints
	schedules := group.Group("/schedules")
	{
		schedules.GET("", handlers.GetSchedules)
		schedules.GET("/:id", handlers.GetMatchup)
	}

	// Transactions endpoints
	transactions := group.Group("/transactions")
	{
		transactions.GET("", handlers.GetTransactions)
		transactions.GET("/draft-picks", handlers.GetDraftPicks)
	}

	// Expected wins endpoints
	expectedWins := group.Group("/expected-wins")
	{
		expectedWins.GET("/weekly/:year", handlers.GetWeeklyExpectedWins)
		expectedWins.GET("/season/:year", handlers.GetSeasonExpectedWins)
		expectedWins.GET("/rankings/:year", handlers.GetSeasonRankings)
		expectedWins.GET("/luck/:year", handlers.GetLuckDistribution)
	}

	// League metadata
	group.GET("/years", handlers.GetLeagueYears)

	// Simulation endpoints
	sim := group.Group("/simulations")
	{
		sim.GET("/stats", handlers.GetStats)
	}
}

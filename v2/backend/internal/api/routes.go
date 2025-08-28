package api

import (
	"backend/internal/api/handlers"

	"github.com/gin-gonic/gin"
)

// SetupRouter configures the API routes
func SetupRouter(r *gin.Engine) {
	// Health check endpoint
	r.GET("/api/health", handlers.HealthCheck)

	// API routes group
	api := r.Group("/api")
	{
		// Teams endpoints
		teams := api.Group("/teams")
		{
			teams.GET("", handlers.GetTeams)
			teams.GET("/:id", handlers.GetTeamByID)
			teams.GET("/all-time-expected-wins", handlers.GetAllTimeExpectedWins)
		}

		// Players endpoints
		players := api.Group("/players")
		{
			players.GET("", handlers.GetPlayers)
			players.GET("/:id", handlers.GetPlayerByID)
			players.GET("/stats", handlers.GetPlayerStats)
		}

		schedules := api.Group("/schedules")
		{
			schedules.GET("", handlers.GetSchedules)
			schedules.GET("/:id", handlers.GetMatchup)
		}

		transactions := api.Group("/transactions")
		{
			transactions.GET("", handlers.GetTransactions)
			transactions.GET("/draft-picks", handlers.GetDraftPicks)
		}

		// Leagues endpoints
		leagues := api.Group("/leagues")
		{
			// League-wide properties
			leagues.GET("/years", handlers.GetLeagueYears)
			
			// Expected wins endpoints
			leagues.GET("/:id/expected-wins/weekly/:year", handlers.GetWeeklyExpectedWins)
			leagues.GET("/:id/expected-wins/season/:year", handlers.GetSeasonExpectedWins)
			leagues.GET("/:id/expected-wins/rankings/:year", handlers.GetSeasonRankings)
			leagues.GET("/:id/expected-wins/luck/:year", handlers.GetLuckDistribution)

			// NOTE: Removed POST endpoints for expected wins recalculation
			// Use ETL scripts instead: --calculate-expected-wins flag
		}

		// Teams endpoints (additional expected wins routes)
		teams.GET("/:id/expected-wins/:year", handlers.GetTeamProgression)

		// Simulation endpoints
		sim := api.Group("/simulations")
		{
			sim.GET("/stats", handlers.GetStats)
			// sim.POST("/run", handlers.RunSimulation)
			// sim.GET("/results/:id", handlers.GetSimulationResults)
		}
	}
}

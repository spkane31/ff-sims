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
		}

		// // Leagues endpoints
		// leagues := api.Group("/leagues")
		// {
		// 	leagues.GET("", handlers.GetLeagues)
		// 	leagues.GET("/:id", handlers.GetLeagueByID)
		// 	leagues.POST("", middleware.AuthRequired(), handlers.CreateLeague)
		// 	leagues.PUT("/:id", middleware.AuthRequired(), handlers.UpdateLeague)
		// 	leagues.GET("/:id/standings", handlers.GetLeagueStandings)
		// }

		// // Simulation endpoints
		// sim := api.Group("/simulations")
		// {
		// 	sim.POST("/run", middleware.AuthRequired(), handlers.RunSimulation)
		// 	sim.GET("/results/:id", handlers.GetSimulationResults)
		// }
	}
}

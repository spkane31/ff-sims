package api

import (
	"backend/internal/api/handlers"

	"github.com/gin-gonic/gin"
)

func SetupRouter(r *gin.Engine) {
	r.GET("/api/health", handlers.HealthCheck)

	v1 := r.Group("/api/v1")

	leagues := v1.Group("/leagues")
	leagues.GET("", handlers.GetLeagues)
	leagues.GET("/:leagueId", handlers.GetLeague)

	leagueScoped := leagues.Group("/:leagueId")
	leagueScoped.GET("/years", handlers.GetLeagueYears)
	leagueScoped.GET("/teams", handlers.GetTeams)
	leagueScoped.GET("/teams/all-time-expected-wins", handlers.GetAllTimeExpectedWins)
	leagueScoped.GET("/teams/standings/:year", handlers.GetCurrentSeasonStandings)
	leagueScoped.GET("/teams/:teamId", handlers.GetTeamByID)
	leagueScoped.GET("/teams/:teamId/expected-wins/:year", handlers.GetTeamProgression)
	leagueScoped.GET("/schedules", handlers.GetSchedules)
	leagueScoped.GET("/schedules/:matchupId", handlers.GetMatchup)
	leagueScoped.GET("/transactions", handlers.GetTransactions)
	leagueScoped.GET("/transactions/draft-picks", handlers.GetDraftPicks)
	leagueScoped.GET("/simulations/stats", handlers.GetStats)
	leagueScoped.GET("/expected-wins/weekly/:year", handlers.GetWeeklyExpectedWins)
	leagueScoped.GET("/expected-wins/season/:year", handlers.GetSeasonExpectedWins)
	leagueScoped.GET("/expected-wins/rankings/:year", handlers.GetSeasonRankings)
	leagueScoped.GET("/expected-wins/luck/:year", handlers.GetLuckDistribution)

	v1.GET("/transactions", handlers.GetTransactions)

	players := v1.Group("/players")
	players.GET("", handlers.GetPlayers)
	players.GET("/stats", handlers.GetPlayerStats)
	players.GET("/:id", handlers.GetPlayerByID)

	sleeper := v1.Group("/sleeper")
	sleeper.GET("/stats", handlers.GetSleeperStats)
	sleeper.GET("/trades", handlers.GetSleeperTrades)
	sleeper.GET("/transactions", handlers.GetSleeperTransactions)
	sleeper.GET("/drafts", handlers.GetSleeperDrafts)

	admin := v1.Group("/admin")
	admin.GET("/backlog", handlers.GetAdminBacklog)
	admin.GET("/segments", handlers.GetAdminSegments)
	admin.GET("/database-size", handlers.GetAdminDatabaseSize)
}

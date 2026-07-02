package workflows

type UserDiscoveryParams struct {
	UserID string
}

type LeagueSyncParams struct {
	LeagueID       string
	LastLegFetched *int
}

type SyncWeekStatsParams struct {
	Season string
}

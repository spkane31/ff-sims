package workflows

import "backend/internal/models"

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

type SegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}

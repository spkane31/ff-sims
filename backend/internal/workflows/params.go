package workflows

import "backend/internal/models"

type UserDiscoveryParams struct {
	UserID string
}

type SyncWeekStatsParams struct {
	Season string
}

type SegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}

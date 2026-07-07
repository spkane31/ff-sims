package workflows

import "backend/internal/models"

type SyncWeekStatsParams struct {
	Season string
}

type SegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}

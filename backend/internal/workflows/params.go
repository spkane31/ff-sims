package workflows

import "backend/internal/models"

type SyncWeekStatsParams struct {
	Season string
}

type SegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}

// DraftSyncReport summarizes one DraftSyncDispatcher run.
type DraftSyncReport struct {
	LeaguesProcessed int
	LeaguesFailed    int
}

// PlayerSyncReport summarizes one PlayerDatabaseSyncWorkflow run. Identity
// counts are only populated on success — SyncPlayerIdentities' conflicts (if
// any) make the whole workflow return an error instead (see
// PlayerDatabaseSyncWorkflow), so there's no "conflicts" field here to keep
// stale-looking at zero on the happy path.
type PlayerSyncReport struct {
	PlayersUpserted int

	IdentitiesScanned int
	IdentitiesLinked  int
	IdentitiesCreated int
}

// WeekStatsReport summarizes a SyncWeekStats (or WeekStatsSyncDispatcher) run.
type WeekStatsReport struct {
	WeeksFetched    int
	PlayersUpserted int
}

// ADPRollupDispatchReport summarizes one ADPRollupDispatcher run. Child
// workflows are fire-and-forget (ParentClosePolicy: ABANDON), so this counts
// segments scheduled, not completed.
type ADPRollupDispatchReport struct {
	SegmentsScheduled int
}

// SegmentADPReport summarizes one SegmentSeasonADPRollupWorkflow run.
type SegmentADPReport struct {
	PlayersUpserted int
}

// BackfillReport summarizes one ArchiveBackfillWorkflow execution (not the
// full backfill lifetime across ContinueAsNew hops).
type BackfillReport struct {
	LeaguesReplicated      int
	TransactionsReplicated int
	DraftHeadersReplicated int
	DraftPicksReplicated   int
}

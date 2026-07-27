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

// PlayerSyncReport summarizes one PlayerDatabaseSyncWorkflow run.
// IdentityConflictDetails is one line per sleeper_players row
// SyncPlayerIdentities couldn't resolve automatically (see its doc
// comment) — carried all the way into this successful result specifically
// so they're easy to find directly in the workflow's own output, rather
// than requiring a dig through activity failure history.
type PlayerSyncReport struct {
	PlayersUpserted int

	IdentitiesScanned       int
	IdentitiesLinked        int
	IdentitiesCreated       int
	IdentitiesConflicts     int
	IdentityConflictDetails []string
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

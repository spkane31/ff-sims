package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// PlayerDatabaseSyncWorkflow runs the daily full Sleeper player DB sync, then
// mirrors sleeper_players into players' sleeper_id column (SyncPlayerIdentities)
// so /players/:id can be resolved from a Sleeper player ID as well as an
// ESPN one. The second step runs right after the first on the same
// worker/schedule specifically so it always sees that run's freshest
// sleeper_players data — no separate scheduling needed.
func PlayerDatabaseSyncWorkflow(ctx workflow.Context) (PlayerSyncReport, error) {
	psa := &activities.PlayerSyncActivities{}

	// 15-minute StartToCloseTimeout and 30s HeartbeatTimeout to detect worker
	// crashes during the large (~5MB) API response processing.
	fetchCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	})
	var res activities.PlayerSyncResult
	if err := workflow.ExecuteActivity(fetchCtx, psa.FetchAndUpsertAllPlayers).Get(ctx, &res); err != nil {
		return PlayerSyncReport{}, err
	}

	// SyncPlayerIdentities does its own DB round-trip per sleeper_players row
	// (no bulk upsert) across up to ~12k rows, so it gets a longer
	// StartToCloseTimeout than the fetch step above; its 30s HeartbeatTimeout
	// is safe because it now heartbeats every playerIdentityHeartbeatInterval
	// rows internally, not just once per up-to-500-row batch.
	identityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	})
	var identityRes activities.PlayerIdentitySyncResult
	// A non-nil error here is a genuine unexpected failure (DB connectivity,
	// etc.) — SyncPlayerIdentities reports its own conflicts as data in
	// identityRes, not as an error, specifically so this workflow can still
	// succeed and carry them in its own result below (see
	// PlayerSyncReport.IdentityConflictDetails) rather than failing every
	// day forever over a handful of permanently-ambiguous Sleeper rows.
	if err := workflow.ExecuteActivity(identityCtx, psa.SyncPlayerIdentities).Get(ctx, &identityRes); err != nil {
		return PlayerSyncReport{}, err
	}

	return PlayerSyncReport{
		PlayersUpserted:         res.PlayersUpserted,
		IdentitiesScanned:       identityRes.Scanned,
		IdentitiesLinked:        identityRes.Linked,
		IdentitiesCreated:       identityRes.Created,
		IdentitiesConflicts:     identityRes.Conflicts,
		IdentityConflictDetails: identityRes.ConflictDetails,
	}, nil
}

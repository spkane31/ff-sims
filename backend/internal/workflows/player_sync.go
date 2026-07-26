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
// Uses a 15-minute StartToCloseTimeout and 30s HeartbeatTimeout to detect worker crashes
// during the large (~5MB) API response processing.
func PlayerDatabaseSyncWorkflow(ctx workflow.Context) (PlayerSyncReport, error) {
	psa := &activities.PlayerSyncActivities{}
	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    10 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumAttempts:    3,
		},
	})
	var res activities.PlayerSyncResult
	if err := workflow.ExecuteActivity(actCtx, psa.FetchAndUpsertAllPlayers).Get(ctx, &res); err != nil {
		return PlayerSyncReport{}, err
	}

	var identityRes activities.PlayerIdentitySyncResult
	if err := workflow.ExecuteActivity(actCtx, psa.SyncPlayerIdentities).Get(ctx, &identityRes); err != nil {
		// err here is a genuine-conflict report (see SyncPlayerIdentities'
		// doc comment), not a transient failure — surface it rather than
		// swallowing it, so it shows up as an actionable Temporal workflow
		// failure for manual follow-up. The per-batch writes it made before
		// hitting the conflicts already committed regardless of this
		// workflow-level failure, and its own partial counts are visible on
		// the SyncPlayerIdentities activity's own completed-with-error event
		// in Temporal's history even though the workflow result below is
		// discarded, matching how FetchAndUpsertAllPlayers' failure is
		// handled above.
		return PlayerSyncReport{}, err
	}

	return PlayerSyncReport{
		PlayersUpserted:   res.PlayersUpserted,
		IdentitiesScanned: identityRes.Scanned,
		IdentitiesLinked:  identityRes.Linked,
		IdentitiesCreated: identityRes.Created,
	}, nil
}

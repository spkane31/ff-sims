package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
)

// PlayerDatabaseSyncWorkflow runs the daily full Sleeper player DB sync.
// Uses a 15-minute StartToCloseTimeout and 30s HeartbeatTimeout to detect worker crashes
// during the large (~5MB) API response processing.
func PlayerDatabaseSyncWorkflow(ctx workflow.Context) error {
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
	return workflow.ExecuteActivity(actCtx, psa.FetchAndUpsertAllPlayers).Get(ctx, nil)
}

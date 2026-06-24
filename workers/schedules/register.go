package schedules

import (
	"context"
	"log"
	"time"

	"go.temporal.io/sdk/client"

	"workers/internal/workflows"
)

// Register creates the Temporal schedules for the Sleeper workers.
// If a schedule already exists it is left unchanged (idempotent).
func Register(ctx context.Context, c client.Client) error {
	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-discovery-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 15 * time.Minute},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.DiscoveryBatchDispatcher,
			TaskQueue: workflows.TaskQueueDiscovery,
		},
	}); err != nil {
		return err
	}

	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-player-sync-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 3}},
					Minute: []client.ScheduleRange{{Start: 0}},
					Comment: "Daily at 03:00 UTC",
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.PlayerDatabaseSyncWorkflow,
			TaskQueue: workflows.TaskQueuePlayerSync,
		},
	})
}

func upsert(ctx context.Context, c client.Client, opts client.ScheduleOptions) error {
	_, err := c.ScheduleClient().Create(ctx, opts)
	if err != nil {
		// Schedule already exists — leave it unchanged
		log.Printf("schedule %q already exists, skipping", opts.ID)
	}
	return nil
}

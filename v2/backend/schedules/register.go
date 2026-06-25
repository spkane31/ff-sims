package schedules

import (
	"context"
	"log"
	"time"

	"go.temporal.io/sdk/client"

	"backend/internal/workflows"
)

// Register creates the Temporal schedules for the Sleeper workers.
// If a schedule already exists it is left unchanged (idempotent).
func Register(ctx context.Context, c client.Client) error {
	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-discovery-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 10 * time.Minute},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.DiscoveryBatchDispatcher,
			TaskQueue: workflows.TaskQueueDiscovery,
		},
	}); err != nil {
		return err
	}

	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-draft-sync-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 10 * time.Minute},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.DraftSyncDispatcher,
			TaskQueue: workflows.TaskQueueDrafts,
		},
	}); err != nil {
		return err
	}

	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-transaction-sync-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 10 * time.Minute},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:  workflows.TransactionSyncDispatcher,
			TaskQueue: workflows.TaskQueueTransactions,
		},
	}); err != nil {
		return err
	}

	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-player-sync-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					DayOfWeek: []client.ScheduleRange{{Start: 2}}, // Tuesday
					Hour:      []client.ScheduleRange{{Start: 8}}, // 03:00 EST (UTC-5)
					Minute:    []client.ScheduleRange{{Start: 0}},
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

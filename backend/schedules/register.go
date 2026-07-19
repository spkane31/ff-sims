package schedules

import (
	"context"
	"log"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"

	"backend/internal/workflows"
)

// Register creates the Temporal schedules for the Sleeper workers. If a
// schedule already exists it is left unchanged (idempotent). archiveEnabled
// gates the ADP rollup schedule — registering it when no worker polls its
// queue would just be a schedule that fires and returns a "no worker
// available" fail, forever, on a queue nobody's listening to.
func Register(ctx context.Context, c client.Client, archiveEnabled bool) error {
	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-discovery-schedule",
		Spec: client.ScheduleSpec{
			Intervals: []client.ScheduleIntervalSpec{
				{Every: 10 * time.Minute},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:                 workflows.DiscoveryBatchDispatcher,
			TaskQueue:                workflows.TaskQueueDiscovery,
			WorkflowExecutionTimeout: 60 * time.Minute,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
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
			Workflow:                 workflows.DraftSyncDispatcher,
			TaskQueue:                workflows.TaskQueueDrafts,
			WorkflowExecutionTimeout: 60 * time.Minute,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
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
			Workflow:                 workflows.TransactionSyncDispatcher,
			TaskQueue:                workflows.TaskQueueTransactions,
			WorkflowExecutionTimeout: 60 * time.Minute,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
	}); err != nil {
		return err
	}

	if err := upsert(ctx, c, client.ScheduleOptions{
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
			Workflow:                 workflows.PlayerDatabaseSyncWorkflow,
			TaskQueue:                workflows.TaskQueuePlayerSync,
			WorkflowExecutionTimeout: 60 * time.Minute,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
	}); err != nil {
		return err
	}

	if err := upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-week-stats-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 9}}, // 04:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:                 workflows.WeekStatsSyncDispatcher,
			TaskQueue:                workflows.TaskQueueWeekStats,
			WorkflowExecutionTimeout: 60 * time.Minute,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
	}); err != nil {
		return err
	}

	if !archiveEnabled {
		return nil
	}

	return upsert(ctx, c, client.ScheduleOptions{
		ID: "sleeper-adp-rollup-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 11}}, // 06:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
				},
			},
		},
		Action: &client.ScheduleWorkflowAction{
			Workflow:                 workflows.ADPRollupDispatcher,
			TaskQueue:                workflows.TaskQueueADP,
			WorkflowExecutionTimeout: 30 * time.Minute,
		},
		Overlap: enums.SCHEDULE_OVERLAP_POLICY_BUFFER_ONE,
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

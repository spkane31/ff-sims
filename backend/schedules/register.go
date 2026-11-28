package schedules

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"

	"backend/internal/workflows"
)

// Register pauses the retired draft schedule and creates schedules for the
// remaining Sleeper workers. Existing non-retired schedules are left unchanged.
// archiveEnabled
// gates the ADP rollup schedule — registering it when no worker polls its
// queue would just be a schedule that fires and returns a "no worker
// available" fail, forever, on a queue nobody's listening to.
func Register(ctx context.Context, c client.Client, archiveEnabled bool) error {
	// Removing Create alone would leave the existing server-side schedule
	// running. Pause it on every startup; never recreate it on a fresh host.
	err := c.ScheduleClient().GetHandle(ctx, "sleeper-draft-sync-schedule").Pause(ctx, client.SchedulePauseOptions{
		Note: "Retired: Sleeper league and transaction writes are disabled",
	})
	var notFound *serviceerror.NotFound
	if err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("pause retired draft schedule: %w", err)
	}

	// Daily, not weekly: the same bulk response feeds the player status
	// snapshot, and injury designations change during the week. Once a day
	// stays within Sleeper's guidance for /v1/players/nfl. upsertSpec rewrites
	// the spec of an already-deployed schedule, which upsert would leave on
	// its old weekly cadence.
	if err := upsertSpec(ctx, c, client.ScheduleOptions{
		ID: "sleeper-player-sync-schedule",
		Spec: client.ScheduleSpec{
			Calendars: []client.ScheduleCalendarSpec{
				{
					Hour:   []client.ScheduleRange{{Start: 8}}, // 03:00 EST (UTC-5)
					Minute: []client.ScheduleRange{{Start: 0}},
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

// upsertSpec creates the schedule, or replaces the spec of an existing one so
// cadence changes reach already-deployed schedules. Other settings on an
// existing schedule (paused state, notes, action) are left unchanged.
func upsertSpec(ctx context.Context, c client.Client, opts client.ScheduleOptions) error {
	_, err := c.ScheduleClient().Create(ctx, opts)
	if err == nil {
		return nil
	}
	if !errors.Is(err, temporal.ErrScheduleAlreadyRunning) {
		return fmt.Errorf("create schedule %q: %w", opts.ID, err)
	}
	spec := opts.Spec
	err = c.ScheduleClient().GetHandle(ctx, opts.ID).Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(in client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			s := in.Description.Schedule
			s.Spec = &spec
			return &client.ScheduleUpdate{Schedule: &s}, nil
		},
	})
	if err != nil {
		return fmt.Errorf("update schedule %q spec: %w", opts.ID, err)
	}
	log.Printf("schedule %q already exists, updated spec", opts.ID)
	return nil
}

func upsert(ctx context.Context, c client.Client, opts client.ScheduleOptions) error {
	_, err := c.ScheduleClient().Create(ctx, opts)
	if err != nil {
		// Schedule already exists — leave it unchanged
		log.Printf("schedule %q already exists, skipping", opts.ID)
	}
	return nil
}

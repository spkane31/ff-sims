package schedules

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/mocks"
	"go.temporal.io/sdk/temporal"
)

// The deployed player sync schedule predates the daily cadence, so Register
// must rewrite its spec instead of skipping it like the other schedules.
func TestRegisterUpdatesExistingPlayerSyncScheduleToDaily(t *testing.T) {
	c := &mocks.Client{}
	sc := &mocks.ScheduleClient{}
	draft := &mocks.ScheduleHandle{}
	player := &mocks.ScheduleHandle{}
	c.On("ScheduleClient").Return(sc)
	sc.On("GetHandle", mock.Anything, "sleeper-draft-sync-schedule").Return(draft)
	draft.On("Pause", mock.Anything, mock.Anything).Return(nil)
	sc.On("Create", mock.Anything, mock.Anything).Return(nil, temporal.ErrScheduleAlreadyRunning)
	sc.On("GetHandle", mock.Anything, "sleeper-player-sync-schedule").Return(player)

	paused := true
	weekly := client.ScheduleSpec{Calendars: []client.ScheduleCalendarSpec{{
		DayOfWeek: []client.ScheduleRange{{Start: 2}},
		Hour:      []client.ScheduleRange{{Start: 8}},
	}}}
	var updated *client.ScheduleUpdate
	player.On("Update", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		opts := args.Get(1).(client.ScheduleUpdateOptions)
		var err error
		updated, err = opts.DoUpdate(client.ScheduleUpdateInput{Description: client.ScheduleDescription{
			Schedule: client.Schedule{Spec: &weekly, State: &client.ScheduleState{Paused: paused}},
		}})
		require.NoError(t, err)
	}).Return(nil)

	require.NoError(t, Register(context.Background(), c, false))
	require.NotNil(t, updated)
	cal := updated.Schedule.Spec.Calendars
	require.Len(t, cal, 1)
	require.Empty(t, cal[0].DayOfWeek, "runs every day")
	require.Equal(t, []client.ScheduleRange{{Start: 8}}, cal[0].Hour)
	require.True(t, updated.Schedule.State.Paused, "operator pause is preserved")
}

func TestRegisterRetiresDraftSchedule(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pauseErr error
	}{
		{name: "existing"},
		{name: "absent", pauseErr: serviceerror.NewNotFound("no draft schedule")},
		{name: "pause failed", pauseErr: errors.New("permission denied")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &mocks.Client{}
			sc := &mocks.ScheduleClient{}
			h := &mocks.ScheduleHandle{}
			c.On("ScheduleClient").Return(sc)
			sc.On("GetHandle", mock.Anything, "sleeper-draft-sync-schedule").Return(h)
			h.On("Pause", mock.Anything, mock.Anything).Return(tc.pauseErr)
			var created []string
			sc.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				created = append(created, args.Get(1).(client.ScheduleOptions).ID)
			}).Return(h, nil)

			err := Register(context.Background(), c, true)
			if tc.name == "pause failed" {
				require.ErrorIs(t, err, tc.pauseErr)
				require.Empty(t, created)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, []string{"sleeper-player-sync-schedule", "sleeper-week-stats-schedule", "sleeper-adp-rollup-schedule"}, created)
			}
			h.AssertCalled(t, "Pause", mock.Anything, mock.Anything)
		})
	}
}

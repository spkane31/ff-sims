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
)

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

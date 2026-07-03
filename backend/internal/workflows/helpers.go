package workflows

import (
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	TaskQueueDiscovery    = "sleeper-discovery"
	TaskQueueDrafts       = "sleeper-drafts"
	TaskQueueTransactions = "sleeper-transactions"
	TaskQueuePlayerSync   = "sleeper-player-sync"
	TaskQueueWeekStats    = "sleeper-week-stats"
	TaskQueueADP          = "sleeper-adp"
	BatchSize             = 10
	SyncBatchSize         = 400
)

var defaultActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 5 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		InitialInterval:    5 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumAttempts:    3,
	},
}

func isNotFound(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "NOT_FOUND"
}

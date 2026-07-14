package workflows

import (
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
	TaskQueueArchive      = "archive-maintenance"

	// MaxDispatchIterations bounds a sync dispatcher's claim loop so one run's
	// event history stays small; the schedule picks up any remainder.
	// At the default draft/transaction sync tuning (2 batches x 100 leagues),
	// that's 25 x 2 x 100 = 5k leagues per run — deliberately tuned down from
	// 25k so we can observe per-batch completion times against the schedule
	// interval before scaling back up.
	MaxDispatchIterations = 25
)

var defaultActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 15 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		InitialInterval:    5 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumAttempts:    3,
	},
}

// batchActivityOptions suit long-running batch activities that heartbeat:
// generous StartToClose for a batch under rate limiting, tight
// HeartbeatTimeout so a dead worker is detected in minutes and the retry
// re-processes only unstamped leagues/users.
var batchActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Minute,
	HeartbeatTimeout:    4 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		InitialInterval:    5 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumAttempts:    3,
	},
}

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
	// 25 iterations × 4 batches × 250 leagues = 25k leagues per run.
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
// generous StartToClose for a 250-league batch under rate limiting, tight
// HeartbeatTimeout so a dead worker is detected in minutes and the retry
// re-processes only unstamped leagues.
var batchActivityOptions = workflow.ActivityOptions{
	StartToCloseTimeout: 30 * time.Minute,
	HeartbeatTimeout:    4 * time.Minute,
	RetryPolicy: &temporal.RetryPolicy{
		InitialInterval:    5 * time.Second,
		BackoffCoefficient: 2.0,
		MaximumAttempts:    3,
	},
}

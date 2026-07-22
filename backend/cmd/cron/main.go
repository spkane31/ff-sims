// cmd/cron is a generic scheduled-job runner: it takes a job name and a
// max-duration, runs the matching job under a deadline context, and exits.
// It's the replacement entrypoint for pipelines migrated off Temporal — see
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md.
// Registers "discovery", "lifetime-counts", and "transactions"; adding
// another (draft-sync, etc., when its turn comes) is a matter of registering
// another function in the registry built in main(), not restructuring this
// file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"

	"backend/internal/activities"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/discoverycron"
	"backend/internal/sleeper"
	"backend/internal/statscron"
	"backend/internal/transactioncron"
)

// buildID identifies the commit this binary was built from. Set via
// -ldflags "-X main.buildID=<git short SHA>" in the worker host's build
// paths (deploy/worker-host/{deploy,setup}.sh), matching cmd/worker's
// convention.
var buildID = "dev"

var errUnknownJob = errors.New("unknown job")

// resolveJob looks up name in registry, returning errUnknownJob (wrapped
// with the attempted name) if it isn't registered.
func resolveJob(registry map[string]func(context.Context) error, name string) (func(context.Context) error, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, errUnknownJob)
	}
	return fn, nil
}

// jobFailed decides whether a discovery Report represents a silent total
// failure that should make the process exit non-zero, rather than a
// legitimate no-op. RunDiscovery/RunPool never return a Go error for "the
// queue was empty" or "the DB was unreachable" — both look identical at the
// Report level unless we check ClaimErrors: zero processed/failed items is
// expected when there's genuinely nothing to claim, but zero processed/
// failed items *and* at least one claim error means every attempt to talk
// to the database failed for the whole run, which must not exit clean.
func jobFailed(report discoverycron.Report) error {
	totalProgress := report.UsersProcessed + report.UsersFailed + report.LeaguesProcessed + report.LeaguesFailed
	claimErrors := report.UserClaimErrors + report.LeagueClaimErrors
	if totalProgress == 0 && claimErrors > 0 {
		return fmt.Errorf("discovery made no progress and saw %d claim error(s) (userClaimErrors=%d, leagueClaimErrors=%d): treating as failure",
			claimErrors, report.UserClaimErrors, report.LeagueClaimErrors)
	}
	return nil
}

// txnJobFailed is transactionCronFailed's analog for transactioncron.Report
// — see jobFailed's doc comment for the reasoning (zero progress plus a
// nonzero claim-error count means the run couldn't talk to the database at
// all, not that the backlog was genuinely empty).
func txnJobFailed(report transactioncron.Report) error {
	totalProgress := report.LeaguesProcessed + report.LeaguesFailed
	if totalProgress == 0 && report.ClaimErrors > 0 {
		return fmt.Errorf("transaction sync made no progress and saw %d claim error(s): treating as failure", report.ClaimErrors)
	}
	return nil
}

func main() {
	jobName := flag.String("job", "", "job to run (see registry in main.go)")
	maxDuration := flag.Duration("max-duration", 0, "hard deadline for the job, e.g. 50m")
	flag.Parse()

	if *jobName == "" {
		log.Fatal("missing required -job flag")
	}
	if *maxDuration <= 0 {
		log.Fatal("missing required -max-duration flag (e.g. -max-duration=50m)")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	// Archive is only needed by "lifetime-counts" (for its purge-immune
	// transactions/drafts metrics — see statscron.RunSnapshot). Its migrations
	// are applied by cmd/worker at startup on the same host, not here.
	if cfg.ArchiveDB.Enabled() {
		if err := database.InitializeArchive(cfg); err != nil {
			log.Fatalf("archive db connect: %v", err)
		}
	} else {
		log.Println("ARCHIVE_DATABASE_URL not set — archive database disabled")
	}

	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}
	dfa := &activities.DataFetchActivities{DB: database.DB, Archive: database.Archive, Sleeper: sc}

	registry := map[string]func(context.Context) error{
		"discovery": func(ctx context.Context) error {
			report, err := discoverycron.RunDiscovery(ctx, da, discoverycron.LoadConfig())
			if err != nil {
				return err
			}
			return jobFailed(report)
		},
		"lifetime-counts": func(ctx context.Context) error {
			_, err := statscron.RunSnapshot(ctx, database.DB, database.Archive)
			return err
		},
		"transactions": func(ctx context.Context) error {
			report, err := transactioncron.RunTransactionSync(ctx, dfa, transactioncron.LoadConfig())
			if err != nil {
				return err
			}
			return txnJobFailed(report)
		},
	}

	fn, err := resolveJob(registry, *jobName)
	if err != nil {
		log.Fatalf("resolve job: %v", err)
	}

	log.Printf("cmd/cron starting: job=%s max_duration=%s build_id=%s", *jobName, *maxDuration, buildID)
	ctx, cancel := context.WithTimeout(context.Background(), *maxDuration)
	defer cancel()

	if err := fn(ctx); err != nil {
		log.Fatalf("job %s failed: %v", *jobName, err)
	}
	log.Printf("cmd/cron finished: job=%s", *jobName)
}

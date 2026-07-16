// cmd/cron is a generic scheduled-job runner: it takes a job name and a
// max-duration, runs the matching job under a deadline context, and exits.
// It's the replacement entrypoint for pipelines migrated off Temporal — see
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md.
// Currently registers exactly one job ("discovery"); adding another
// (draft-sync, transaction-sync, etc., when their turn comes) is a matter of
// registering another function in the registry built in main(), not
// restructuring this file.
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

	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}

	registry := map[string]func(context.Context) error{
		"discovery": func(ctx context.Context) error {
			_, err := discoverycron.RunDiscovery(ctx, da, discoverycron.LoadConfig())
			return err
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

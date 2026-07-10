package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/contrib/sysinfo"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"backend/internal/activities"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/dbmigrate"
	"backend/internal/helpers"
	"backend/internal/sleeper"
	"backend/internal/workflows"
	archivemigrations "backend/migrations/archive"
	"backend/schedules"
)

// buildID identifies the commit this binary was built from. Set via
// -ldflags "-X main.buildID=<git short SHA>" in the worker host's build paths
// (deploy/worker-host/{deploy,setup}.sh) — the only place cmd/worker is built.
var buildID = "dev"

// promoteOnStart marks this build as the deployment's promoting fleet: on startup
// it calls SetCurrentVersion to make its own build the deployment's Current
// Version. Set to "true" via -ldflags in deploy/worker-host/{deploy,setup}.sh — the
// worker host is the only fleet that runs cmd/worker (DigitalOcean's Dockerfile
// only builds cmd/server and the Python ESPN worker), so it's always the sole
// promoter.
//
// This used to be asymmetric across two fleets (DigitalOcean promoting, the
// Raspberry Pi joining) and had to stay baked in at build time rather than
// configured via an env var, since a second promoting fleet racing
// SetCurrentVersion could make stale code current and reintroduce the
// non-determinism problem Worker Deployment Versioning exists to prevent. Now
// that only one fleet ever runs this binary, that risk no longer applies, but
// the flag stays build-time-baked rather than defaulted to "true" in source: an
// accidental second worker-host build (e.g. a dev running cmd/worker locally
// without ldflags) should join the deployment, not fight to promote a dev build.
var promoteOnStart = "false"

// deploymentName identifies the Worker Deployment used by the worker host fleet.
const deploymentName = "ff-sims-worker"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	if cfg.ArchiveDB.Enabled() {
		if err := dbmigrate.Run(cfg.ArchiveDB.ConnectionString, archivemigrations.FS, "up", nil); err != nil {
			log.Fatalf("archive db migrate: %v", err)
		}
		if err := database.InitializeArchive(cfg); err != nil {
			log.Fatalf("archive db connect: %v", err)
		}
	} else {
		log.Println("ARCHIVE_DATABASE_URL not set — archive database disabled")
	}

	c, err := client.Dial(temporalClientOptions())
	if err != nil {
		log.Fatalf("temporal dial: %v", err)
	}
	defer c.Close()

	if err := schedules.Register(context.Background(), c, cfg.ArchiveDB.Enabled()); err != nil {
		log.Fatalf("register schedules: %v", err)
	}

	deploymentVersion := worker.WorkerDeploymentVersion{
		DeploymentName: deploymentName,
		BuildID:        buildID,
	}
	deploymentOpts := worker.DeploymentOptions{
		UseVersioning:             true,
		Version:                   deploymentVersion,
		DefaultVersioningBehavior: workflow.VersioningBehaviorPinned,
	}
	log.Printf("worker deployment: name=%s build_id=%s", deploymentName, buildID)

	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}
	dfa := &activities.DataFetchActivities{DB: database.DB, Sleeper: sc}
	psa := &activities.PlayerSyncActivities{DB: database.DB, Sleeper: sc}
	wsa := &activities.WeekStatsActivities{DB: database.DB, Sleeper: sc}
	aa := &activities.ADPRollupActivities{DB: database.DB}

	// Discovery worker: DiscoveryBatchDispatcher (claim-drain batch model)
	dw := worker.New(c, workflows.TaskQueueDiscovery, worker.Options{
		DeploymentOptions: deploymentOpts,
		SysInfoProvider:   sysinfo.SysInfoProvider(),
	})
	dw.RegisterWorkflow(workflows.DiscoveryBatchDispatcher)
	dw.RegisterActivity(da)

	// The sync queues (drafts, transactions) are I/O-bound, and Temporal task
	// distribution is pull-based: the fleet with more free activity slots and
	// pollers takes a larger share of the queue. These are env-tunable so the
	// worker host (idles well under 10% CPU on this workload) can be scaled up
	// as needed.
	//
	//	WORKER_ACTIVITY_SLOTS    max concurrent activities per sync queue (default 100)
	//	WORKER_ACTIVITY_POLLERS  activity task pollers per sync queue (0 = SDK default)
	syncWorkerOptions := worker.Options{
		MaxConcurrentActivityExecutionSize: max(helpers.GetEnv("WORKER_ACTIVITY_SLOTS", 100), 1),
		MaxConcurrentWorkflowTaskPollers:   10,
		DeploymentOptions:                  deploymentOpts,
		SysInfoProvider:                    sysinfo.SysInfoProvider(),
	}
	if pollers := helpers.GetEnv("WORKER_ACTIVITY_POLLERS", 0); pollers > 0 {
		syncWorkerOptions.MaxConcurrentActivityTaskPollers = pollers
	}
	log.Printf("sync worker tuning: activity_slots=%d activity_pollers=%d (0 = SDK default)",
		syncWorkerOptions.MaxConcurrentActivityExecutionSize, syncWorkerOptions.MaxConcurrentActivityTaskPollers)

	// Drafts worker: DraftSyncDispatcher (claim-drain batch model)
	draftsw := worker.New(c, workflows.TaskQueueDrafts, syncWorkerOptions)
	draftsw.RegisterWorkflow(workflows.DraftSyncDispatcher)
	draftsw.RegisterActivity(dfa)

	// Transactions worker: TransactionSyncDispatcher (claim-drain batch model)
	transactionsw := worker.New(c, workflows.TaskQueueTransactions, syncWorkerOptions)
	transactionsw.RegisterWorkflow(workflows.TransactionSyncDispatcher)
	transactionsw.RegisterActivity(dfa)

	// Player sync worker: PlayerDatabaseSyncWorkflow
	psw := worker.New(c, workflows.TaskQueuePlayerSync, worker.Options{
		DeploymentOptions: deploymentOpts,
		SysInfoProvider:   sysinfo.SysInfoProvider(),
	})
	psw.RegisterWorkflow(workflows.PlayerDatabaseSyncWorkflow)
	psw.RegisterActivity(psa)

	// Week stats worker: WeekStatsSyncDispatcher + SyncWeekStats
	wsw := worker.New(c, workflows.TaskQueueWeekStats, worker.Options{
		DeploymentOptions: deploymentOpts,
		SysInfoProvider:   sysinfo.SysInfoProvider(),
	})
	wsw.RegisterWorkflow(workflows.WeekStatsSyncDispatcher)
	wsw.RegisterWorkflow(workflows.SyncWeekStats)
	wsw.RegisterActivity(wsa)

	// ADP worker: ADPRollupDispatcher + SegmentSeasonADPRollupWorkflow
	adpw := worker.New(c, workflows.TaskQueueADP, worker.Options{
		MaxConcurrentActivityExecutionSize: 50,
		MaxConcurrentWorkflowTaskPollers:   10,
		DeploymentOptions:                  deploymentOpts,
		SysInfoProvider:                    sysinfo.SysInfoProvider(),
	})
	adpw.RegisterWorkflow(workflows.ADPRollupDispatcher)
	adpw.RegisterWorkflow(workflows.SegmentSeasonADPRollupWorkflow)
	adpw.RegisterActivity(aa)

	workers := []worker.Worker{dw, draftsw, transactionsw, psw, wsw, adpw}
	if cfg.ArchiveDB.Enabled() {
		sa := &activities.ScavengerActivities{Cloud: database.DB, Archive: database.Archive}
		aw := worker.New(c, workflows.TaskQueueArchive, worker.Options{
			DeploymentOptions: deploymentOpts,
			SysInfoProvider:   sysinfo.SysInfoProvider(),
		})
		aw.RegisterWorkflow(workflows.ScavengerDispatcher)
		aw.RegisterWorkflow(workflows.ArchiveBackfillWorkflow)
		aw.RegisterActivity(sa)
		workers = append(workers, aw)
	}
	for _, w := range workers {
		if err := w.Start(); err != nil {
			log.Fatalf("worker start: %v", err)
		}
	}
	defer func() {
		for _, w := range workers {
			w.Stop()
		}
	}()

	if promoteOnStart == "true" {
		go promoteDeploymentVersion(context.Background(), c, deploymentVersion)
	}

	log.Println("Temporal workers started — waiting for SIGINT/SIGTERM")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
}

// promoteDeploymentVersion sets this process's build as the deployment's current
// version, so new workflow executions route to it. The version isn't registered
// with the server until a worker has polled at least once, so early attempts may
// fail — retry with capped backoff indefinitely rather than giving up. A single
// missed window here previously meant the deployment never got a Current Version
// at all, so every new workflow execution across every task queue was created but
// could never be assigned to a worker (this is exactly what happened the first
// time versioning shipped: the promotion never landed, and nothing retried after
// the old one-minute budget expired). Only called when promoteOnStart is "true"
// (the worker host's build); a build without the flag never calls this and just
// joins whatever version its build produces.
func promoteDeploymentVersion(ctx context.Context, c client.Client, version worker.WorkerDeploymentVersion) {
	handle := c.WorkerDeploymentClient().GetHandle(version.DeploymentName)
	backoff := time.Second
	for {
		_, err := handle.SetCurrentVersion(ctx, client.WorkerDeploymentSetCurrentVersionOptions{
			BuildID: version.BuildID,
		})
		if err == nil {
			log.Printf("promoted deployment %s to current version build_id=%s", version.DeploymentName, version.BuildID)
			return
		}
		log.Printf("promoting deployment %s to build_id=%s failed, retrying in %s: %v", version.DeploymentName, version.BuildID, backoff, err)
		select {
		case <-ctx.Done():
			log.Printf("giving up promoting deployment %s to build_id=%s: %v", version.DeploymentName, version.BuildID, ctx.Err())
			return
		case <-time.After(backoff):
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

// temporalClientOptions returns client.Options configured for either Temporal Cloud
// (when TEMPORAL_NAMESPACE_ENDPOINT is set) or a local dev server.
//
// Temporal Cloud env vars:
//
//	TEMPORAL_NAMESPACE_ENDPOINT  e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
//	TEMPORAL_NAMESPACE           e.g. ff-sims.b3i2g
//	TEMPORAL_API_KEY             API key for authentication
//
// Local dev env vars (fallbacks):
//
//	TEMPORAL_HOST       default localhost:7233
//	TEMPORAL_NAMESPACE  default "default"
func temporalClientOptions() client.Options {
	if endpoint := os.Getenv("TEMPORAL_NAMESPACE_ENDPOINT"); endpoint != "" {
		opts := client.Options{
			HostPort:  endpoint,
			Namespace: os.Getenv("TEMPORAL_NAMESPACE"),
			ConnectionOptions: client.ConnectionOptions{
				TLS: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Temporal Cloud uses self-signed cert on tmprl-test.cloud
			},
		}
		if apiKey := os.Getenv("TEMPORAL_API_KEY"); apiKey != "" {
			opts.Credentials = client.NewAPIKeyStaticCredentials(apiKey)
		}
		return opts
	}
	return client.Options{
		HostPort:  helpers.GetEnv("TEMPORAL_HOST", "localhost:7233"),
		Namespace: helpers.GetEnv("TEMPORAL_NAMESPACE", "default"),
	}
}

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
	"backend/internal/sleeper"
	"backend/internal/workflows"
	"backend/schedules"
)

// buildID identifies the commit this binary was built from. Set via
// -ldflags "-X main.buildID=<git short SHA>" in all build paths (Dockerfile,
// deploy/raspberry-pi/{deploy,setup}.sh). Both fleets built from the same commit
// must produce the identical string so they share one deployment version.
var buildID = "dev"

// promoteOnStart marks this build as the deployment's promoting fleet: on startup
// it calls SetCurrentVersion to make its own build the deployment's Current
// Version. Set to "true" via -ldflags only in the Dockerfile (DigitalOcean);
// deploy/raspberry-pi/{deploy,setup}.sh never set it, so Pi builds default to
// "false" and just join whatever version they produce.
//
// This must stay asymmetric and baked in at build time rather than configured
// via an env var. If both fleets promoted, whichever happened to poll and call
// SetCurrentVersion last would win — including a Pi mid self-update still
// running a stale build — silently making stale code current and reintroducing
// the non-determinism problem Worker Deployment Versioning exists to prevent.
// It was previously an opt-in env var (TEMPORAL_PROMOTE_ON_START) that only
// DigitalOcean's App Platform config was supposed to set — a fact about which
// fleet promotes that lived entirely outside this repo, unreviewed and easy to
// forget. Baking the flag into the Dockerfile removes that dependency: DO always
// builds from this file, so promotion eligibility can no longer silently go
// unconfigured the way GIT_SHA did.
var promoteOnStart = "false"

// deploymentName identifies the Worker Deployment shared by the DigitalOcean and
// Raspberry Pi worker fleets.
const deploymentName = "ff-sims-worker"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("db connect: %v", err)
	}

	c, err := client.Dial(temporalClientOptions())
	if err != nil {
		log.Fatalf("temporal dial: %v", err)
	}
	defer c.Close()

	if err := schedules.Register(context.Background(), c); err != nil {
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

	// Discovery worker: DiscoveryBatchDispatcher + UserDiscoveryWorkflow
	dw := worker.New(c, workflows.TaskQueueDiscovery, worker.Options{
		DeploymentOptions: deploymentOpts,
		SysInfoProvider:   sysinfo.SysInfoProvider(),
	})
	dw.RegisterWorkflow(workflows.DiscoveryBatchDispatcher)
	dw.RegisterWorkflow(workflows.UserDiscoveryWorkflow)
	dw.RegisterActivity(da)

	// Drafts worker: DraftSyncDispatcher + LeagueDraftSyncWorkflow
	draftsw := worker.New(c, workflows.TaskQueueDrafts, worker.Options{
		MaxConcurrentActivityExecutionSize: 100,
		MaxConcurrentWorkflowTaskPollers:   10,
		DeploymentOptions:                  deploymentOpts,
		SysInfoProvider:                    sysinfo.SysInfoProvider(),
	})
	draftsw.RegisterWorkflow(workflows.DraftSyncDispatcher)
	draftsw.RegisterWorkflow(workflows.LeagueDraftSyncWorkflow)
	draftsw.RegisterActivity(dfa)

	// Transactions worker: TransactionSyncDispatcher + LeagueTransactionSyncWorkflow
	transactionsw := worker.New(c, workflows.TaskQueueTransactions, worker.Options{
		MaxConcurrentActivityExecutionSize: 100,
		MaxConcurrentWorkflowTaskPollers:   10,
		DeploymentOptions:                  deploymentOpts,
		SysInfoProvider:                    sysinfo.SysInfoProvider(),
	})
	transactionsw.RegisterWorkflow(workflows.TransactionSyncDispatcher)
	transactionsw.RegisterWorkflow(workflows.LeagueTransactionSyncWorkflow)
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

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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
// (DigitalOcean); the Pi never calls this and just joins whatever version its
// build produces.
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
		HostPort:  getEnv("TEMPORAL_HOST", "localhost:7233"),
		Namespace: getEnv("TEMPORAL_NAMESPACE", "default"),
	}
}

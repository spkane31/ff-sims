package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"backend/internal/activities"
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/sleeper"
	"backend/internal/workflows"
	"backend/schedules"
)

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

	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: database.DB, Sleeper: sc}
	dfa := &activities.DataFetchActivities{DB: database.DB, Sleeper: sc}
	psa := &activities.PlayerSyncActivities{DB: database.DB, Sleeper: sc}

	// Discovery worker: DiscoveryBatchDispatcher + UserDiscoveryWorkflow
	dw := worker.New(c, workflows.TaskQueueDiscovery, worker.Options{})
	dw.RegisterWorkflow(workflows.DiscoveryBatchDispatcher)
	dw.RegisterWorkflow(workflows.UserDiscoveryWorkflow)
	dw.RegisterActivity(da)

	// Data worker: LeagueSyncWorkflow
	dataw := worker.New(c, workflows.TaskQueueData, worker.Options{})
	dataw.RegisterWorkflow(workflows.LeagueSyncWorkflow)
	dataw.RegisterActivity(dfa)

	// Player sync worker: PlayerDatabaseSyncWorkflow
	psw := worker.New(c, workflows.TaskQueuePlayerSync, worker.Options{})
	psw.RegisterWorkflow(workflows.PlayerDatabaseSyncWorkflow)
	psw.RegisterActivity(psa)

	workers := []worker.Worker{dw, dataw, psw}
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

// temporalClientOptions returns client.Options configured for either Temporal Cloud
// (when TEMPORAL_NAMESPACE_ENDPOINT is set) or a local dev server.
//
// Temporal Cloud env vars:
//   TEMPORAL_NAMESPACE_ENDPOINT  e.g. ff-sims.b3i2g.tmprl-test.cloud:7233
//   TEMPORAL_NAMESPACE           e.g. ff-sims.b3i2g
//   TEMPORAL_API_KEY             API key for authentication
//
// Local dev env vars (fallbacks):
//   TEMPORAL_HOST       default localhost:7233
//   TEMPORAL_NAMESPACE  default "default"
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

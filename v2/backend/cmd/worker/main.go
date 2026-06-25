package main

import (
	"context"
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

	temporalHost := getEnv("TEMPORAL_HOST", "localhost:7233")
	temporalNS := getEnv("TEMPORAL_NAMESPACE", "default")

	c, err := client.Dial(client.Options{
		HostPort:  temporalHost,
		Namespace: temporalNS,
	})
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

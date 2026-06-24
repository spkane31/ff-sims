package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"workers/internal/activities"
	internaldb "workers/internal/db"
	"workers/internal/sleeper"
	"workers/internal/workflows"
	"workers/schedules"
)

func main() {
	_ = godotenv.Load()

	db, err := internaldb.Connect()
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := internaldb.RunMigrations(sqlDB); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	c, err := client.Dial(client.Options{
		HostPort:  getEnv("TEMPORAL_HOST", "localhost:7233"),
		Namespace: getEnv("TEMPORAL_NAMESPACE", "default"),
	})
	if err != nil {
		log.Fatalf("temporal dial: %v", err)
	}
	defer c.Close()

	if err := schedules.Register(context.Background(), c); err != nil {
		log.Fatalf("register schedules: %v", err)
	}

	sc := sleeper.New()
	da := &activities.DiscoveryActivities{DB: db, Sleeper: sc}
	dfa := &activities.DataFetchActivities{DB: db, Sleeper: sc}
	psa := &activities.PlayerSyncActivities{DB: db, Sleeper: sc}

	// Discovery worker: handles DiscoveryBatchDispatcher + UserDiscoveryWorkflow
	dw := worker.New(c, workflows.TaskQueueDiscovery, worker.Options{})
	dw.RegisterWorkflow(workflows.DiscoveryBatchDispatcher)
	dw.RegisterWorkflow(workflows.UserDiscoveryWorkflow)
	dw.RegisterActivity(da)

	// Data worker: handles LeagueSyncWorkflow
	dataw := worker.New(c, workflows.TaskQueueData, worker.Options{})
	dataw.RegisterWorkflow(workflows.LeagueSyncWorkflow)
	dataw.RegisterActivity(dfa)

	// Player sync worker: handles PlayerDatabaseSyncWorkflow
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

	log.Println("workers started — waiting for SIGINT/SIGTERM")
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

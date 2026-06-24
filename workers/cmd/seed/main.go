package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"gorm.io/gorm/clause"

	internaldb "workers/internal/db"
	"workers/internal/models"
	"workers/internal/sleeper"
	"workers/internal/workflows"
)

func main() {
	_ = godotenv.Load()

	var username string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Bootstrap Sleeper data collection from a seed username",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), username)
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Sleeper username to seed from (required)")
	_ = cmd.MarkFlagRequired("username")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, username string) error {
	db, err := internaldb.Connect()
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}

	sc := sleeper.New()
	user, err := sc.GetUser(ctx, username)
	if err != nil {
		return fmt.Errorf("resolve user %q: %w", username, err)
	}
	log.Printf("resolved: %s → %s", user.Username, user.UserID)

	row := models.SleeperUser{
		SleeperUserID: user.UserID,
		Username:      user.Username,
		DisplayName:   user.DisplayName,
		Avatar:        user.Avatar,
	}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	c, err := client.Dial(client.Options{
		HostPort:  getEnv("TEMPORAL_HOST", "localhost:7233"),
		Namespace: getEnv("TEMPORAL_NAMESPACE", "default"),
	})
	if err != nil {
		return fmt.Errorf("temporal dial: %w", err)
	}
	defer c.Close()

	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "seed-" + user.UserID,
		TaskQueue: workflows.TaskQueueDiscovery,
	}, workflows.UserDiscoveryWorkflow, user.UserID)
	if err != nil {
		return fmt.Errorf("start workflow: %w", err)
	}
	log.Printf("started workflow: %s / %s", run.GetID(), run.GetRunID())

	if err := run.Get(ctx, nil); err != nil {
		return fmt.Errorf("workflow failed: %w", err)
	}
	log.Printf("seed complete — scheduler will take over from here")
	return nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

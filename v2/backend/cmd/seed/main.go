package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"go.temporal.io/sdk/client"
	"gorm.io/gorm/clause"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/sleeper"
	"backend/internal/workflows"
)

func main() {
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
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := database.Initialize(cfg); err != nil {
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
	if err := database.DB.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	c, err := client.Dial(temporalClientOptions())
	if err != nil {
		return fmt.Errorf("temporal dial: %w", err)
	}
	defer c.Close()

	wfRun, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        "seed-" + user.UserID,
		TaskQueue: workflows.TaskQueueDiscovery,
	}, workflows.UserDiscoveryWorkflow, workflows.UserDiscoveryParams{UserID: user.UserID})
	if err != nil {
		return fmt.Errorf("start workflow: %w", err)
	}
	log.Printf("started workflow: %s / %s", wfRun.GetID(), wfRun.GetRunID())

	if err := wfRun.Get(ctx, nil); err != nil {
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

func temporalClientOptions() client.Options {
	if endpoint := os.Getenv("TEMPORAL_NAMESPACE_ENDPOINT"); endpoint != "" {
		opts := client.Options{
			HostPort:  endpoint,
			Namespace: os.Getenv("TEMPORAL_NAMESPACE"),
			ConnectionOptions: client.ConnectionOptions{
				TLS: &tls.Config{},
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

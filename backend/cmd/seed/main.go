package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"gorm.io/gorm/clause"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/sleeper"
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

	// The seeded user has last_fetched_at NULL, so the cron discovery job's
	// claim query (internal/discoverycron) picks it up first on its next run —
	// no need to trigger discovery here, just make sure the job is running.
	log.Printf("seed complete — the discovery cron job will pick up %s on its next run", user.UserID)
	return nil
}

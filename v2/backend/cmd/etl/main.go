package main

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/etl"
	"backend/internal/logging"
	"backend/internal/models"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var (
	dataDir          string
	skipExpectedWins bool
	processYear      uint
	leagueExternalID string
	platform         string
)

// TODO(temporal): migrate to Temporal workflow — see issue for Temporal migration
func main() {
	// Root command
	rootCmd := &cobra.Command{
		Use:   "etl",
		Short: "ETL service for fantasy football simulations",
		Long:  "Extract, Transform, Load service for processing fantasy football data",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Infof("ETL service started")
			logging.Infof("Using data directory: %s", dataDir)
		},
	}

	// Add global flags
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "./data", "Directory containing data files")
	rootCmd.PersistentFlags().StringVar(&leagueExternalID, "league-id", "", "Platform-assigned league ID (e.g. 345674)")
	rootCmd.PersistentFlags().StringVar(&platform, "platform", "ESPN", "Fantasy platform: ESPN, Sleeper, Yahoo")

	// Upload command
	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload data to the database",
		Long:  "Process and upload data files to the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			if leagueExternalID == "" {
				return fmt.Errorf("--league-id is required")
			}
			leagueID, err := resolveLeagueID(leagueExternalID, platform)
			if err != nil {
				return err
			}
			return etl.UploadWithOptions(dataDir, leagueID, !skipExpectedWins)
		},
	}
	uploadCmd.Flags().BoolVar(&skipExpectedWins, "skip-expected-wins", false, "Skip expected wins calculations during ETL")

	// Expected wins command
	xwinsCmd := &cobra.Command{
		Use:   "xwins",
		Short: "Calculate expected wins",
		Long:  "Calculate expected wins for fantasy football teams based on their performance",
		RunE: func(cmd *cobra.Command, args []string) error {
			if leagueExternalID == "" {
				return fmt.Errorf("--league-id is required")
			}
			leagueID, err := resolveLeagueID(leagueExternalID, platform)
			if err != nil {
				return err
			}
			if processYear > 0 {
				logging.Infof("Running expected wins calculation for year %d only", processYear)
			} else {
				logging.Infof("Running expected wins calculation for all years")
			}
			return etl.ProcessExpectedWinsWithYear(leagueID, processYear)
		},
	}
	xwinsCmd.Flags().UintVar(&processYear, "year", 0, "Specific year to process for expected wins (0 = all years, starting with most recent)")

	// Add commands to root
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(xwinsCmd)

	rootCmd.SilenceUsage = true
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	if err := rootCmd.Execute(); err != nil {
		logging.Errorf("Error executing command: %v", err)
		os.Exit(1)
	}
}

// resolveLeagueID initialises the database and looks up the internal league ID
// by (external_id, platform). Returns an error if the league is not found.
func resolveLeagueID(externalID, plt string) (uint, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, fmt.Errorf("error loading configuration: %w", err)
	}
	if err := database.Initialize(cfg); err != nil {
		return 0, fmt.Errorf("error initialising database: %w", err)
	}

	var league models.League
	err = database.DB.Where("external_id = ? AND platform = ?", externalID, plt).First(&league).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, fmt.Errorf("league not found: external_id=%s platform=%s — run the leagues API to create it first", externalID, plt)
		}
		return 0, fmt.Errorf("error looking up league: %w", err)
	}

	logging.Infof("Resolved league %q (platform=%s) to internal ID %d", externalID, plt, league.ID)
	return league.ID, nil
}

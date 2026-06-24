package main

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/etl"
	"backend/internal/logging"
	"backend/internal/models"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var (
	dataDir          string
	skipExpectedWins bool
	processYear      uint
	uploadYear       uint
	leagueExternalID string
	platform         string
)

type leaguePath struct {
	externalID string
	year       uint
	dir        string
}

// discoverLeaguePaths walks dataDir two levels deep to find {leagueExternalID}/{year}/
// subdirectories. leagueFilter and yearFilter are optional (zero value = no filter).
func discoverLeaguePaths(dataDir, leagueFilter string, yearFilter uint) ([]leaguePath, error) {
	leagueDirs, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read data directory %s: %w", dataDir, err)
	}

	var paths []leaguePath
	for _, entry := range leagueDirs {
		if !entry.IsDir() {
			continue
		}
		externalID := entry.Name()
		if leagueFilter != "" && externalID != leagueFilter {
			continue
		}

		yearDirs, err := os.ReadDir(filepath.Join(dataDir, externalID))
		if err != nil {
			return nil, fmt.Errorf("failed to read league directory %s: %w", externalID, err)
		}

		for _, yearEntry := range yearDirs {
			if !yearEntry.IsDir() {
				continue
			}
			yearStr := yearEntry.Name()
			parsed, err := strconv.ParseUint(yearStr, 10, 32)
			if err != nil {
				logging.Warnf("Skipping non-year directory %q in league %s", yearStr, externalID)
				continue
			}
			year := uint(parsed)
			if yearFilter > 0 && year != yearFilter {
				continue
			}
			paths = append(paths, leaguePath{
				externalID: externalID,
				year:       year,
				dir:        filepath.Join(dataDir, externalID, yearStr),
			})
		}
	}
	return paths, nil
}

// TODO(temporal): migrate to Temporal workflow — see issue for Temporal migration
func main() {
	rootCmd := &cobra.Command{
		Use:   "etl",
		Short: "ETL service for fantasy football simulations",
		Long:  "Extract, Transform, Load service for processing fantasy football data",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Infof("ETL service started")
			logging.Infof("Using data directory: %s", dataDir)
		},
	}

	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", "./data", "Directory containing data files")
	rootCmd.PersistentFlags().StringVar(&leagueExternalID, "league-id", "", "Platform-assigned league ID filter (e.g. 345674); optional for upload")
	rootCmd.PersistentFlags().StringVar(&platform, "platform", "ESPN", "Fantasy platform: ESPN, Sleeper, Yahoo")

	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload data to the database",
		Long:  "Process and upload data files to the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := discoverLeaguePaths(dataDir, leagueExternalID, uploadYear)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				logging.Warnf("No league/year directories found under %s", dataDir)
				return nil
			}
			for _, p := range paths {
				leagueID, err := resolveLeagueID(p.externalID, platform)
				if err != nil {
					return err
				}
				logging.Infof("Processing league %s (internal ID %d), year %d from %s",
					p.externalID, leagueID, p.year, p.dir)
				if err := etl.UploadWithOptions(p.dir, leagueID, !skipExpectedWins); err != nil {
					return err
				}
			}
			return nil
		},
	}
	uploadCmd.Flags().BoolVar(&skipExpectedWins, "skip-expected-wins", false, "Skip expected wins calculations during ETL")
	uploadCmd.Flags().UintVar(&uploadYear, "year", 0, "Specific year to process (0 = all years)")

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
		if err != gorm.ErrRecordNotFound {
			return 0, fmt.Errorf("error looking up league: %w", err)
		}
		league = models.League{
			Name:       fmt.Sprintf("%s League %s", plt, externalID),
			Platform:   plt,
			ExternalID: externalID,
		}
		if createErr := database.DB.Create(&league).Error; createErr != nil {
			return 0, fmt.Errorf("error creating league: %w", createErr)
		}
		logging.Infof("Created new league %q (platform=%s) with internal ID %d", externalID, plt, league.ID)
	} else {
		logging.Infof("Resolved league %q (platform=%s) to internal ID %d", externalID, plt, league.ID)
	}
	return league.ID, nil
}

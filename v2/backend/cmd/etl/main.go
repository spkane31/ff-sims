package main

import (
	"backend/internal/etl"
	"backend/internal/logging"
	"os"

	"github.com/spf13/cobra"
)

var (
	dataDir          string
	skipExpectedWins bool
	processYear      uint
)

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

	// Upload command
	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload data to the database",
		Long:  "Process and upload data files to the database",
		Run: func(cmd *cobra.Command, args []string) {
			// Determine if we should calculate expected wins
			doCalculateExpectedWins := !skipExpectedWins

			// Run normal ETL with expected wins flag
			if err := etl.UploadWithOptions(dataDir, doCalculateExpectedWins); err != nil {
				logging.Errorf("Failed to upload data: %v", err)
				os.Exit(1)
			}
		},
	}
	uploadCmd.Flags().BoolVar(&skipExpectedWins, "skip-expected-wins", false, "Skip expected wins calculations during ETL")

	// Expected wins command
	xwinsCmd := &cobra.Command{
		Use:   "xwins",
		Short: "Calculate expected wins",
		Long:  "Calculate expected wins for fantasy football teams based on their performance",
		Run: func(cmd *cobra.Command, args []string) {
			if processYear > 0 {
				logging.Infof("Running expected wins calculation for year %d only", processYear)
			} else {
				logging.Infof("Running expected wins calculation for all years")
			}
			if err := etl.ProcessExpectedWinsWithYear(processYear); err != nil {
				logging.Errorf("Failed to calculate expected wins: %v", err)
				os.Exit(1)
			}
		},
	}
	xwinsCmd.Flags().UintVar(&processYear, "year", 0, "Specific year to process for expected wins (0 = all years, starting with most recent)")

	// Add commands to root
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(xwinsCmd)

	rootCmd.SilenceUsage = true // Suppress usage message on error

	// Suppress the completion built in command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Execute
	if err := rootCmd.Execute(); err != nil {
		logging.Errorf("Error executing command: %v", err)
		os.Exit(1)
	}
}

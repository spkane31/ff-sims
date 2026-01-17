package main

import (
	"backend/internal/etl"
	"backend/internal/logging"
	"os"

	"github.com/spf13/cobra"
)

var (
	dataDir           string
	multipleLeagues   bool
	refreshPlayerData bool
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

	// Upload command
	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload data to the database",
		Long:  "Process and upload data files to the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			return etl.NewUpload(etl.NewUploadOptions{
				Directory:         dataDir,
				MultipleLeagues:   multipleLeagues,
				RefreshPlayerData: refreshPlayerData,
			})
		},
	}
	uploadCmd.Flags().StringVar(&dataDir, "directory", "./data", "Directory containing data files")
	uploadCmd.Flags().BoolVar(&multipleLeagues, "multiple-leagues", false, "There are multiple leagues in the data directory")
	uploadCmd.Flags().BoolVar(&refreshPlayerData, "refresh-players", false, "Force refresh player data from Sleeper API and update local JSON file")

	// Add commands to root
	rootCmd.AddCommand(uploadCmd)

	rootCmd.SilenceUsage = true // Suppress usage message on error

	// Suppress the completion built in command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Execute
	if err := rootCmd.Execute(); err != nil {
		logging.Errorf("Error executing command: %v", err)
		os.Exit(1)
	}
}

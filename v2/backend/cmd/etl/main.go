package main

import (
	"backend/internal/etl"
	"backend/internal/logging"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	dataDir string
)

func main() {
	// Root command
	rootCmd := &cobra.Command{
		Use:   "etl",
		Short: "ETL service for fantasy football simulations",
		Long:  "Extract, Transform, Load service for processing fantasy football data",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Info("ETL service started")
			logging.Info(fmt.Sprintf("Using data directory: %s", dataDir))
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
			if err := etl.Upload(dataDir); err != nil {
				logging.Error(fmt.Sprintf("Failed to upload data: %v", err))
				os.Exit(1)
			}
		},
	}

	// Add commands to root
	rootCmd.AddCommand(uploadCmd)

	rootCmd.SilenceUsage = true // Suppress usage message on error

	// Suppress the completion built in command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Execute
	if err := rootCmd.Execute(); err != nil {
		logging.Error(fmt.Sprintf("Error executing command: %v", err))
		os.Exit(1)
	}
}

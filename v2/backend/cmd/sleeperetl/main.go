package main

import (
	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/logging"
	"backend/internal/models"
	"backend/internal/sleeper"
	"os"
	"time"

	"github.com/spf13/cobra"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	// Root command
	rootCmd := &cobra.Command{
		Use:   "sleeper",
		Short: "ETL service for fantasy football simulations",
		Long:  "Extract, Transform, Load service for processing fantasy football data",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logging.Infof("Sleeper ETL service started")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return findAllLeagues(100)
		},
	}

	rootCmd.SilenceUsage = true // Suppress usage message on error

	// Suppress the completion built in command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Execute
	if err := rootCmd.Execute(); err != nil {
		logging.Errorf("Error executing command: %v", err)
		os.Exit(1)
	}
}

func findAllLeagues(processCount int) error {
	if err := database.Initialize(&config.Config{DB: config.DBConfig{ConnectionString: os.Getenv("DATABASE_URL")}}); err != nil {
		logging.Errorf("Failed to initialize database: %v", err)
		return err
	}

	db := database.DB

	// Use OnConflict to handle duplicates - do nothing if already exists
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sleeper_id"}},
		DoNothing: true,
	}).Create(&models.SleeperLeague{SleeperID: "1257055775478521856"}).Error
	if err != nil {
		logging.Errorf("Failed to insert initial league: %v", err)
		return err
	}
	client := sleeper.New()

	// Get batch of leagues to process
	// leagues := []models.SleeperLeague{}
	// err = db.Model(&models.SleeperLeague{}).Where("last_scraped IS NULL").Limit(processCount).Find(&leagues).Error
	// if err != nil {
	// 	logging.Errorf("Failed to fetch leagues to process: %v", err)
	// 	return err
	// }

	count := 0
	// Create a transaction for each batch of 5
	for i := 0; i < processCount; i += 5 {
		err = db.Transaction(func(tx *gorm.DB) error {
			// Get the next batch of leagues to scrape
			leagueBatches := []models.SleeperLeague{}
			err := tx.Model(&models.SleeperLeague{}).Where("last_scraped IS NULL").Limit(5).Find(&leagueBatches).Error
			if err != nil {
				return err
			}

			for _, league := range leagueBatches {
				users, err := client.GetUsersInLeague(league.SleeperID)
				if err != nil {
					logging.Errorf("Error fetching users in league %s: %v", league.SleeperID, err)
					return err
				}
				logging.Infof("Fetched %d users in league %s", len(users), league.SleeperID)

				for _, user := range users {
					// Create the user in the database
					err := tx.Clauses(clause.OnConflict{DoNothing: true}).Model(&models.SleeperUser{}).Create(&models.SleeperUser{SleeperID: user.UserID}).Error
					if err != nil {
						logging.Errorf("Failed to upsert user %s: %v", user.UserID, err)
						continue
					}

					userLeagues, err := client.GetAllLeaguesForUser(user.UserID, "nfl", "2025")
					if err != nil {
						logging.Errorf("Error fetching leagues for user %s: %v", user.UserID, err)
						continue
					}
					logging.Debugf("User %s is in %d leagues for 2025 NFL season", user.DisplayName, len(userLeagues))
					for _, league := range userLeagues {
						err := tx.Clauses(clause.OnConflict{DoNothing: true}).Model(&models.SleeperLeague{}).Create(&models.SleeperLeague{SleeperID: league.LeagueID}).Error
						if err != nil {
							logging.Errorf("Failed to upsert league %s: %v", league.LeagueID, err)
							continue
						}
					}
				}

				// Mark league as scraped
				now := time.Now()
				err = tx.Model(&models.SleeperLeague{}).Where("sleeper_id = ?", league.SleeperID).Update("last_scraped", &now).Error
				if err != nil {
					logging.Errorf("Failed to update last_scraped for league %s: %v", league.SleeperID, err)
					return err
				}
				count++
			}
			return nil
		})
		if err != nil {
			logging.Errorf("Transaction failed: %v", err)
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}

	// for count < processCount {
	// 	logging.Infof("%d leagues to search for", processCount-count)
	// 	nextLeague := leagues[count%len(leagues)]

	// 	users, err := client.GetUsersInLeague(nextLeague.SleeperID)
	// 	if err != nil {
	// 		logging.Errorf("Error fetching league: %v", err)
	// 		return err
	// 	}
	// 	logging.Infof("Fetched %d users in league", len(users))

	// 	for _, user := range users {
	// 		logging.Debugf("User ID: %s, Display Name: %s", user.UserID, user.DisplayName)
	// 		// Create the user in the database
	// 		err := db.Clauses(clause.OnConflict{DoNothing: true}).Model(&models.SleeperUser{}).Create(&models.SleeperUser{SleeperID: user.UserID}).Error
	// 		if err != nil {
	// 			logging.Errorf("Failed to upsert user %s: %v", user.UserID, err)
	// 			continue
	// 		}

	// 		userLeagues, err := client.GetAllLeaguesForUser(user.UserID, "nfl", "2025")
	// 		if err != nil {
	// 			logging.Errorf("Error fetching leagues for user %s: %v", user.UserID, err)
	// 			continue
	// 		}
	// 		logging.Debugf("User %s is in %d leagues for 2025 NFL season", user.DisplayName, len(userLeagues))
	// 		for _, league := range userLeagues {
	// 			if !searched[league.LeagueID] {
	// 				allLeagues = append(allLeagues, league.LeagueID)
	// 				logging.Infof("Found new league: %s (Total: %d) (Total Rosters: %d)", league.LeagueID, len(allLeagues), league.TotalRosters)
	// 			}
	// 		}
	// 	}

	// 	count++
	// 	time.Sleep(500 * time.Millisecond)
	// }
	// return nil

	return nil
}

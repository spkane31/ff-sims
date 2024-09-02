package main

import (
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Matchup struct {
	gorm.Model
	HomeTeam, AwayTeam                                     Team
	HomeTeamID, AwayTeamID                                 uint
	HomeTeamESPNID, AwayTeamESPNID                         uint
	HomeTeamFinalScore, AwayTeamFinalScore                 float64
	Completed                                              bool
	HomeTeamESPNProjectedScore, AwayTeamESPNProjectedScore float64
	Week, Year                                             uint
}

type Team struct {
	gorm.Model
	Owner  string
	ESPNID uint
}

type DraftSelection struct {
	gorm.Model
	PlayerName                          string
	PlayerPosition                      string
	TeamID, PlayerID, Round, Pick, Year uint
	OwnerESPNID                         uint
}

func main() {
	// Using Go to generate my database because it's easier for me

	var db *gorm.DB
	connectionString := os.Getenv("COCKROACHDB_URL")

	options := &gorm.Config{}
	options.Logger = logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold: time.Second, // Slow SQL threshold
			LogLevel:      logger.Info, // Log level
			Colorful:      true,        // Colorful display
		},
	)

	db, err := gorm.Open(postgres.Open(connectionString), options)
	if err != nil {
		panic(err)
	}

	if err := db.AutoMigrate(
		&Matchup{},
		&Team{},
		&DraftSelection{},
	); err != nil {
		panic(err)
	}
}

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
	Completed, IsPlayoff                                   bool
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

type BoxScorePlayer struct {
	gorm.Model
	PlayerName, PlayerPosition    string
	Status                        string
	OwnerESPNID                   uint
	TeamID, PlayerID, Week, Year  uint
	ProjectedPoints, ActualPoints float64
	Completed                     bool
}

func main() {
	// Using Go to generate my database because it's easier for me
	start := time.Now()
	defer func() {
		log.Printf("Database generation took %v\n", time.Since(start))
	}()

	var db *gorm.DB
	connectionString := os.Getenv("DATABASE_URL")

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
		&BoxScorePlayer{},
	); err != nil {
		panic(err)
	}
}

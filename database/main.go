package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Matchup struct {
	gorm.Model
	HomeTeam                   Team
	AwayTeam                   Team
	HomeTeamID                 uint
	AwayTeamID                 uint
	HomeTeamESPNID             uint
	AwayTeamESPNID             uint
	HomeTeamFinalScore         float64
	AwayTeamFinalScore         float64
	Completed                  bool
	IsPlayoff                  bool
	HomeTeamESPNProjectedScore float64
	AwayTeamESPNProjectedScore float64
	Week                       uint
	Year                       uint
}

type Team struct {
	gorm.Model
	Owner  string
	ESPNID uint
}

type DraftSelection struct {
	gorm.Model
	PlayerName     string
	PlayerPosition string
	TeamID         uint
	PlayerID       uint
	Round          uint
	Pick           uint
	Year           uint
	OwnerESPNID    uint
}

func (d DraftSelection) String() string {
	return fmt.Sprintf("DraftSelection(PlayerName=%s, PlayerPosition=%s, TeamID=%d, PlayerID=%d, Round=%d, Pick=%d, Year=%d, OwnerESPNID=%d)", d.PlayerName, d.PlayerPosition, d.TeamID, d.PlayerID, d.Round, d.Pick, d.Year, d.OwnerESPNID)
}

type BoxScorePlayer struct {
	gorm.Model
	PlayerName      string
	PlayerPosition  string
	Status          string
	OwnerESPNID     uint
	TeamID          uint
	PlayerID        uint
	Week            uint
	Year            uint
	ProjectedPoints float64
	ActualPoints    float64
}

type Transaction struct {
	gorm.Model
	Date            time.Time
	TeamID          uint
	PlayerID        uint
	TransactionType string
}

type Request struct {
	gorm.Model
	Endpoint   string
	Method     string
	Body       string
	Completed  bool
	UserAgent  string
	RuntimeMS  float64
	StatusCode int
	IsFrontend bool `gorm:"default:false"`
	Timestamp  time.Time
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
		&Request{},
		&Transaction{},
	); err != nil {
		panic(err)
	}
}

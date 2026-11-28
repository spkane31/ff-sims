// Command kickoffs loads NFL kickoff times confirmed from the official NFL
// schedule into nfl_kickoffs. Each season/week in the file replaces what is
// stored for it. Pregame projection captures are scheduled from the
// confirmed rows on the next hourly dispatch.
//
//	go run ./cmd/kickoffs -file week13.csv
//
// CSV format (one game per line):
//
//	season,week,away,home,kickoff,status
//	2026,13,DAL,SEA,2026-12-06T16:25:00-05:00,confirmed
package main

import (
	"flag"
	"log"
	"os"

	_ "github.com/joho/godotenv/autoload"

	"backend/internal/config"
	"backend/internal/database"
	"backend/internal/kickoffs"
)

func main() {
	file := flag.String("file", "", "CSV of kickoffs (season,week,away,home,kickoff,status)")
	source := flag.String("source", "nfl.com", "where the kickoff times were confirmed")
	flag.Parse()
	if *file == "" {
		log.Fatal("-file is required")
	}

	f, err := os.Open(*file)
	if err != nil {
		log.Fatalf("open %s: %v", *file, err)
	}
	defer f.Close()
	rows, err := kickoffs.ParseCSV(f, *source)
	if err != nil {
		log.Fatalf("parse %s: %v", *file, err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("db connect: %v", err)
	}
	if err := kickoffs.Replace(database.DB, rows); err != nil {
		log.Fatalf("load kickoffs: %v", err)
	}
	log.Printf("loaded %d team kickoffs from %s", len(rows), *file)
}

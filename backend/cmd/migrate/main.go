package main

import (
	"flag"
	"io/fs"
	"log"
	"os"

	_ "github.com/joho/godotenv/autoload"

	"backend/internal/dbmigrate"
	"backend/migrations"
	archivemigrations "backend/migrations/archive"
)

func main() {
	dbFlag := flag.String("db", "cloud", "which database to migrate: cloud or archive")
	flag.Parse()

	var dsn string
	var fsys fs.FS
	switch *dbFlag {
	case "cloud":
		dsn = os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("DATABASE_URL is not set")
		}
		fsys = migrations.FS
	case "archive":
		dsn = os.Getenv("ARCHIVE_DATABASE_URL")
		if dsn == "" {
			log.Fatal("ARCHIVE_DATABASE_URL is not set")
		}
		fsys = archivemigrations.FS
	default:
		log.Fatalf("unknown -db value %q (want cloud or archive)", *dbFlag)
	}

	command := "up"
	args := []string{}
	if flag.NArg() > 0 {
		command = flag.Arg(0)
		args = flag.Args()[1:]
	}

	if err := dbmigrate.Run(dsn, fsys, command, args); err != nil {
		log.Fatal(err)
	}
}

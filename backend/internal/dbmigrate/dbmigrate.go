package dbmigrate

import (
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Run applies goose command (e.g. "up", "down", "status") from fsys against
// the database at dsn. Shared by cmd/migrate (manual ops, either DB) and
// cmd/worker (auto-runs archive migrations at startup).
//
// goose.SetBaseFS sets a package-level global in the goose library, so Run
// must not be called concurrently with a different fsys from multiple
// goroutines in the same process. Every current caller is single-shot
// (cmd/migrate) or runs once at startup before serving traffic (cmd/worker),
// so this is safe today; it would need revisiting if that changes.
func Run(dsn string, fsys fs.FS, command string, args []string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Run(command, db, ".", args...); err != nil {
		return fmt.Errorf("goose %s: %w", command, err)
	}
	return nil
}

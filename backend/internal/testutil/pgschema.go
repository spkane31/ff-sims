package testutil

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewPGSchema creates a throwaway schema (named "<prefix>_<random>") on the
// Postgres database at dsn and returns a DSN with search_path pinned to it.
// search_path rides in the DSN itself (not a session SET) so every pooled
// connection — including concurrent goroutines sharing one *gorm.DB — sees
// the same schema. The schema and its contents are dropped via t.Cleanup.
func NewPGSchema(t *testing.T, dsn, prefix string) string {
	t.Helper()
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := fmt.Sprintf("%s_%d", prefix, rand.Int63())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		sqlDB, _ := admin.DB()
		sqlDB.Close()
	})

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "search_path=" + schema
}

// OpenGORM opens a *gorm.DB against scopedDSN (as returned by NewPGSchema)
// with query logging silenced, closing it via t.Cleanup.
func OpenGORM(t *testing.T, scopedDSN string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open postgres (schema-scoped): %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	})
	return db
}

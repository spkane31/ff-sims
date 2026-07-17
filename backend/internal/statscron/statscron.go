// Package statscron implements cmd/cron's "lifetime-counts" job: an hourly
// snapshot of data-scraping table sizes (users/leagues/transactions/drafts),
// written to sleeper_lifetime_counts so growth over time is visible and
// home/admin-page all-time totals survive the scavenger's purge phase. See
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md for
// the cmd/cron job-runner conventions this follows.
package statscron

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
)

// RunSnapshot computes current sizes for the core Sleeper data-scraping
// tables and upserts one row into sleeper_lifetime_counts under the current
// hour's timestamp (SnapshotAt truncated to the hour), building a
// growth-over-time history.
//
// Users and leagues are counted from cloud: those tables are never purged,
// so a live COUNT there is already exact. Transactions and drafts are
// counted from archive (the full-history store) when archive is non-nil:
// cloud's copies of these are purge-trimmed to a hot window and, for drafts,
// mostly bypassed at ingest entirely once the archive DB is configured (see
// syncOneLeagueDrafts in internal/activities/data_fetch.go). When archive is
// nil (no ARCHIVE_DATABASE_URL — e.g. local dev), those three columns are
// left nil on this row rather than written as misleading zeros.
func RunSnapshot(ctx context.Context, cloud, archive *gorm.DB) (models.SleeperLifetimeCount, error) {
	row := models.SleeperLifetimeCount{SnapshotAt: time.Now().UTC().Truncate(time.Hour)}

	var err error
	row.UsersTotal, row.UsersExpanded, row.UsersPending, row.UsersSkipped, err =
		countDiscoveryState(ctx, cloud, "sleeper_users")
	if err != nil {
		return models.SleeperLifetimeCount{}, err
	}
	row.LeaguesTotal, row.LeaguesExpanded, row.LeaguesPending, row.LeaguesSkipped, err =
		countDiscoveryState(ctx, cloud, "sleeper_leagues")
	if err != nil {
		return models.SleeperLifetimeCount{}, err
	}

	if archive != nil {
		var transactionsTotal, tradesCompleted, draftsCompleted int64
		if err := archive.WithContext(ctx).Table("sleeper_transactions").Count(&transactionsTotal).Error; err != nil {
			return models.SleeperLifetimeCount{}, err
		}
		if err := archive.WithContext(ctx).Table("sleeper_transactions").
			Where("type = ? AND status = ?", "trade", "complete").Count(&tradesCompleted).Error; err != nil {
			return models.SleeperLifetimeCount{}, err
		}
		if err := archive.WithContext(ctx).Table("sleeper_drafts").
			Where("status = ?", "complete").Count(&draftsCompleted).Error; err != nil {
			return models.SleeperLifetimeCount{}, err
		}
		row.TransactionsTotal = &transactionsTotal
		row.TradesCompleted = &tradesCompleted
		row.DraftsCompleted = &draftsCompleted
	} else {
		log.Println("statscron: archive DB not configured, leaving transactions/drafts columns nil this run")
	}

	if err := cloud.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "snapshot_at"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"users_total", "users_expanded", "users_pending", "users_skipped",
			"leagues_total", "leagues_expanded", "leagues_pending", "leagues_skipped",
			"transactions_total", "trades_completed", "drafts_completed",
		}),
	}).Create(&row).Error; err != nil {
		return models.SleeperLifetimeCount{}, err
	}

	log.Printf("statscron: wrote snapshot_at=%s users=%d leagues=%d", row.SnapshotAt, row.UsersTotal, row.LeaguesTotal)
	return row, nil
}

// countDiscoveryState counts table's total/expanded/pending/skipped rows —
// the same last_fetched_at/skipped_at discovery-state split
// GetAdminDiscoveryFrontier uses.
func countDiscoveryState(ctx context.Context, db *gorm.DB, table string) (total, expanded, pending, skipped int64, err error) {
	if err = db.WithContext(ctx).Table(table).Count(&total).Error; err != nil {
		return
	}
	if err = db.WithContext(ctx).Table(table).Where("last_fetched_at IS NOT NULL").Count(&expanded).Error; err != nil {
		return
	}
	if err = db.WithContext(ctx).Table(table).Where("last_fetched_at IS NULL AND skipped_at IS NULL").Count(&pending).Error; err != nil {
		return
	}
	err = db.WithContext(ctx).Table(table).Where("skipped_at IS NOT NULL").Count(&skipped).Error
	return
}

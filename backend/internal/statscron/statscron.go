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

// Report summarizes one RunSnapshot call.
type Report struct {
	SnapshotAt time.Time
	Counts     map[string]int64
}

// RunSnapshot computes current sizes for the core Sleeper data-scraping
// tables and upserts them into sleeper_lifetime_counts under the current
// hour's timestamp (SnapshotAt truncated to the hour), building a
// growth-over-time history one row per (hour, metric).
//
// Users and leagues are counted from cloud: those tables are never purged,
// so a live COUNT there is already exact. Transactions and drafts are
// counted from archive (the full-history store) when archive is non-nil:
// cloud's copies of these are purge-trimmed to a hot window and, for drafts,
// mostly bypassed at ingest entirely once the archive DB is configured (see
// syncOneLeagueDrafts in internal/activities/data_fetch.go). When archive is
// nil (no ARCHIVE_DATABASE_URL — e.g. local dev), those three metrics are
// skipped for this snapshot rather than written as misleading zeros.
func RunSnapshot(ctx context.Context, cloud, archive *gorm.DB) (Report, error) {
	snapshotAt := time.Now().UTC().Truncate(time.Hour)
	counts := make(map[string]int64)

	if err := countDiscoveryState(ctx, cloud, "sleeper_users", counts,
		models.LifetimeMetricUsersTotal, models.LifetimeMetricUsersExpanded,
		models.LifetimeMetricUsersPending, models.LifetimeMetricUsersSkipped); err != nil {
		return Report{}, err
	}
	if err := countDiscoveryState(ctx, cloud, "sleeper_leagues", counts,
		models.LifetimeMetricLeaguesTotal, models.LifetimeMetricLeaguesExpanded,
		models.LifetimeMetricLeaguesPending, models.LifetimeMetricLeaguesSkipped); err != nil {
		return Report{}, err
	}

	if archive != nil {
		var transactionsTotal, tradesCompleted, draftsCompleted int64
		if err := archive.WithContext(ctx).Table("sleeper_transactions").Count(&transactionsTotal).Error; err != nil {
			return Report{}, err
		}
		if err := archive.WithContext(ctx).Table("sleeper_transactions").
			Where("type = ? AND status = ?", "trade", "complete").Count(&tradesCompleted).Error; err != nil {
			return Report{}, err
		}
		if err := archive.WithContext(ctx).Table("sleeper_drafts").
			Where("status = ?", "complete").Count(&draftsCompleted).Error; err != nil {
			return Report{}, err
		}
		counts[models.LifetimeMetricTransactionsTotal] = transactionsTotal
		counts[models.LifetimeMetricTradesCompleted] = tradesCompleted
		counts[models.LifetimeMetricDraftsCompleted] = draftsCompleted
	} else {
		log.Println("statscron: archive DB not configured, skipping transactions/drafts metrics this run")
	}

	rows := make([]models.SleeperLifetimeCount, 0, len(counts))
	for metric, count := range counts {
		rows = append(rows, models.SleeperLifetimeCount{SnapshotAt: snapshotAt, Metric: metric, Count: count})
	}
	if err := cloud.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "snapshot_at"}, {Name: "metric"}},
		DoUpdates: clause.AssignmentColumns([]string{"count"}),
	}).Create(&rows).Error; err != nil {
		return Report{}, err
	}

	log.Printf("statscron: wrote %d metrics for snapshot_at=%s", len(rows), snapshotAt)
	return Report{SnapshotAt: snapshotAt, Counts: counts}, nil
}

// countDiscoveryState counts table's total/expanded/pending/skipped rows —
// the same last_fetched_at/skipped_at discovery-state split GetAdminDiscoveryFrontier
// uses — and records them into counts under the given metric names.
func countDiscoveryState(ctx context.Context, db *gorm.DB, table string, counts map[string]int64, totalMetric, expandedMetric, pendingMetric, skippedMetric string) error {
	var total, expanded, pending, skipped int64
	if err := db.WithContext(ctx).Table(table).Count(&total).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Table(table).Where("last_fetched_at IS NOT NULL").Count(&expanded).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Table(table).Where("last_fetched_at IS NULL AND skipped_at IS NULL").Count(&pending).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Table(table).Where("skipped_at IS NOT NULL").Count(&skipped).Error; err != nil {
		return err
	}
	counts[totalMetric] = total
	counts[expandedMetric] = expanded
	counts[pendingMetric] = pending
	counts[skippedMetric] = skipped
	return nil
}

// Package statscron implements cmd/cron's "lifetime-counts" job: an hourly
// snapshot of data-scraping table sizes (users/leagues/transactions/drafts),
// written to sleeper_lifetime_counts so growth over time is visible and
// home/admin-page all-time totals survive the scavenger's purge phase. This
// job also owns keeping the archive DB itself in sync (see archive_sync.go)
// — replicating cloud → archive and purging verified-old cloud rows — since
// sleeper_lifetime_counts is the only consumer of archive's transaction/
// draft counts; that used to be a separately-scheduled Temporal workflow
// (ScavengerDispatcher), which let its schedule silently drift out of sync
// with this job's cadence. See
// docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md for
// the cmd/cron job-runner conventions this follows.
package statscron

import (
	"context"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/activities"
	"backend/internal/models"
)

// RunSnapshot syncs the archive DB (see archive_sync.go's syncArchive) and
// computes current sizes for the core Sleeper data-scraping tables, then
// upserts one row into sleeper_lifetime_counts under the current hour's
// timestamp (SnapshotAt truncated to the hour), building a growth-over-time
// history.
//
// Users and leagues are counted from cloud: those tables are never purged,
// so a live COUNT there is already exact. Transactions and drafts are
// counted from archive (the full-history store) when archive is non-nil:
// cloud's copies of these are purge-trimmed to a hot window and, for drafts,
// mostly bypassed at ingest entirely once the archive DB is configured (see
// syncOneLeagueDrafts in internal/activities/data_fetch.go). When archive is
// nil (no ARCHIVE_DATABASE_URL — e.g. local dev), archive sync is skipped
// and those three columns are left nil on this row rather than written as
// misleading zeros.
//
// The users/leagues/archive branches are independent of each other, so they
// run concurrently: each goroutine below only ever writes its own row
// fields and its own error variable, so there's no shared mutable state
// between them and no need for a mutex.
func RunSnapshot(ctx context.Context, cloud, archive *gorm.DB) (models.SleeperLifetimeCount, error) {
	row := models.SleeperLifetimeCount{SnapshotAt: time.Now().UTC().Truncate(time.Hour)}

	var usersErr, leaguesErr, archiveErr error
	var wg sync.WaitGroup

	wg.Go(func() {
		row.UsersTotal, row.UsersExpanded, row.UsersPending, row.UsersSkipped, usersErr =
			countDiscoveryState(ctx, cloud, "sleeper_users")
	})
	wg.Go(func() {
		row.LeaguesTotal, row.LeaguesExpanded, row.LeaguesPending, row.LeaguesSkipped, leaguesErr =
			countDiscoveryState(ctx, cloud, "sleeper_leagues")
	})
	if archive != nil {
		wg.Go(func() {
			sa := &activities.ScavengerActivities{Cloud: cloud, Archive: archive}
			cfg, err := sa.GetScavengerConfig(ctx)
			if err != nil {
				archiveErr = err
				return
			}
			if _, err := syncArchive(ctx, sa, cfg); err != nil {
				// Only a purge stalled-replication alarm reaches here (see
				// syncArchive's doc comment) — that must fail this run, same
				// as any of the count queries below failing, rather than
				// silently publishing a snapshot against data we know is
				// stuck.
				archiveErr = err
				return
			}

			var transactionsTotal, tradesCompleted, draftsCompleted int64
			// Plain COUNT(*) against archive's full, never-purged
			// sleeper_transactions (unlike the two filtered counts below,
			// no WHERE clause can be indexed) forces a full scan that gets
			// slower as the table grows without bound — it was the actual
			// cause of this job blowing its -max-duration deadline in
			// production. n_live_tup is the same autovacuum-maintained
			// estimate GetAdminDatabaseSize already uses for its "Rows
			// (est.)" column (internal/api/handlers/admin.go); an all-time
			// growth metric doesn't need exact precision.
			if archiveErr = archive.WithContext(ctx).Raw(
				`SELECT COALESCE(n_live_tup, 0) FROM pg_catalog.pg_stat_user_tables WHERE relname = 'sleeper_transactions'`,
			).Scan(&transactionsTotal).Error; archiveErr != nil {
				return
			}
			if archiveErr = archive.WithContext(ctx).Table("sleeper_transactions").
				Where("type = ? AND status = ?", "trade", "complete").Count(&tradesCompleted).Error; archiveErr != nil {
				return
			}
			if archiveErr = archive.WithContext(ctx).Table("sleeper_drafts").
				Where("status = ?", "complete").Count(&draftsCompleted).Error; archiveErr != nil {
				return
			}
			row.TransactionsTotal = &transactionsTotal
			row.TradesCompleted = &tradesCompleted
			row.DraftsCompleted = &draftsCompleted
		})
	} else {
		log.Println("statscron: archive DB not configured, leaving transactions/drafts columns nil this run")
	}

	wg.Wait()
	if usersErr != nil {
		return models.SleeperLifetimeCount{}, usersErr
	}
	if leaguesErr != nil {
		return models.SleeperLifetimeCount{}, leaguesErr
	}
	if archiveErr != nil {
		return models.SleeperLifetimeCount{}, archiveErr
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

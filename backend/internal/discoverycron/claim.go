// Package discoverycron replaces the Temporal-based discovery pipeline
// (workflows.DiscoveryBatchDispatcher / activities.DiscoverUsersBatch) with
// a plain Go implementation driven by a systemd timer instead of a Temporal
// Schedule. See docs/superpowers/specs/2026-07-15-discovery-cron-migration-design.md
// for the design and docs/superpowers/plans/2026-07-15-discovery-cron-migration.md
// for how it was built. Both paths run concurrently against the same claim
// queues for now — this package does not touch the existing Temporal code.
package discoverycron

import (
	"context"

	"gorm.io/gorm"
)

// claimStaleLeaguesSQL atomically claims up to batchSize leagues needing
// discovery's member/detail fetch (mirrors activities.claimStaleUsersSQL's
// shape). Leagues already complete-and-fetched are excluded from the query
// itself — matches activities.leagueFullySynced's condition, but applied
// before claiming rather than after, so a complete league never occupies a
// pool slot at all. season >= '2025' matches
// activities.firstScannedSeason — discovery never creates older rows, but
// this table can carry historical rows from other sources.
const claimStaleLeaguesSQL = `
UPDATE sleeper_leagues SET discovery_claimed_at = now()
WHERE sleeper_league_id IN (
    SELECT sleeper_league_id FROM sleeper_leagues
    WHERE skipped_at IS NULL
      AND season >= '2025'
      AND NOT (status = 'complete' AND last_fetched_at IS NOT NULL)
      AND (discovery_claimed_at IS NULL OR discovery_claimed_at < now() - interval '120 minutes')
    ORDER BY last_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_league_id`

// ClaimStaleLeagues claims up to batchSize leagues for discovery's league
// pool, never-fetched first then oldest. Postgres-only (SKIP LOCKED).
func ClaimStaleLeagues(ctx context.Context, db *gorm.DB, batchSize int) ([]string, error) {
	var ids []string
	if err := db.WithContext(ctx).Raw(claimStaleLeaguesSQL, batchSize).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

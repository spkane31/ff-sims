// Package discoverycron implements cmd/cron's "discovery" job: expands the
// user/league graph via two internal/fdb claim/dispatch/batch pools running
// concurrently — one claiming stale users, one claiming stale leagues.
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

// claimStaleUsersSQL atomically claims up to batchSize stale users for
// discovery (same pattern as the league claim above). FOR UPDATE SKIP LOCKED
// lets concurrent claimers partition the queue without double-claiming, and
// the 120-minute expiry re-queues users claimed by a worker that died
// mid-run. 120 minutes (not 20, unlike the transaction/draft claim columns)
// because this cron path imposes no per-item timeout shorter than that; a
// shorter TTL risked a still-in-flight user being reclaimed and processed a
// second time concurrently. Because ticks claim rather than re-select, a
// stuck cohort can never head-of-line-block the queue. The tradeoff: a
// worker that dies mid-run now leaves its claimed users unclaimable for up
// to 120 minutes before they become re-queueable, a real if minor cost given
// crashes are rare.
const claimStaleUsersSQL = `
UPDATE sleeper_users SET claimed_at = now()
WHERE sleeper_user_id IN (
    SELECT sleeper_user_id FROM sleeper_users
    WHERE skipped_at IS NULL
      AND (claimed_at IS NULL OR claimed_at < now() - interval '120 minutes')
    ORDER BY last_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_user_id`

// ClaimStaleUsers claims up to batchSize users for discovery's user pool,
// never-fetched first then oldest. Postgres-only (SKIP LOCKED).
func ClaimStaleUsers(ctx context.Context, db *gorm.DB, batchSize int) ([]string, error) {
	var ids []string
	if err := db.WithContext(ctx).Raw(claimStaleUsersSQL, batchSize).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

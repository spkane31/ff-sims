// Package transactionage defines the transaction-fetch freshness buckets used
// by both the live admin backlog and its hourly historical snapshots.
package transactionage

import (
	"context"
	"time"

	"gorm.io/gorm"

	"backend/internal/models"
)

var bucketLabels = []string{
	"Never fetched", "0h-3h59m", "4h-7h59m", "8h-11h59m",
	"12h-15h59m", "16h-19h59m", "20h-23h59m", "24h+",
}

// Bucket is one fetch-age category and its league count.
type Bucket struct {
	Label   string
	Leagues int64
}

// Labels returns the canonical fetch-age bucket order.
func Labels() []string {
	return append([]string(nil), bucketLabels...)
}

// Fill orders rows by the canonical bucket labels and zero-fills buckets that
// were absent from the source query.
func Fill(rows []Bucket) []Bucket {
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Label] = row.Leagues
	}

	filled := make([]Bucket, len(bucketLabels))
	for i, label := range bucketLabels {
		filled[i] = Bucket{Label: label, Leagues: counts[label]}
	}
	return filled
}

// CurrentSeason returns the most recent discovered Sleeper league season.
func CurrentSeason(ctx context.Context, db *gorm.DB) (string, error) {
	var season string
	err := db.WithContext(ctx).Model(&models.SleeperLeague{}).
		Select("COALESCE(MAX(season), '')").Scan(&season).Error
	return season, err
}

// Count returns zero-filled fetch-age buckets for one season as of asOf.
// Skipped leagues are deliberately excluded because the transaction worker
// never processes them.
func Count(ctx context.Context, db *gorm.DB, season string, asOf time.Time) ([]Bucket, error) {
	const q = `
		SELECT
			CASE
				WHEN last_transactions_fetched_at IS NULL THEN 'Never fetched'
				WHEN last_transactions_fetched_at > ? THEN '0h-3h59m'
				WHEN last_transactions_fetched_at > ? THEN '4h-7h59m'
				WHEN last_transactions_fetched_at > ? THEN '8h-11h59m'
				WHEN last_transactions_fetched_at > ? THEN '12h-15h59m'
				WHEN last_transactions_fetched_at > ? THEN '16h-19h59m'
				WHEN last_transactions_fetched_at > ? THEN '20h-23h59m'
				ELSE '24h+'
			END AS label,
			COUNT(*) AS leagues
		FROM sleeper_leagues
		WHERE season = ? AND skipped_at IS NULL
		GROUP BY label`

	rows := []Bucket{}
	err := db.WithContext(ctx).Raw(q,
		asOf.Add(-4*time.Hour), asOf.Add(-8*time.Hour), asOf.Add(-12*time.Hour),
		asOf.Add(-16*time.Hour), asOf.Add(-20*time.Hour), asOf.Add(-24*time.Hour),
		season,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return Fill(rows), nil
}

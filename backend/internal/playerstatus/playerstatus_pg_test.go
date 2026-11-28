package playerstatus_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"backend/internal/dbmigrate"
	"backend/internal/playerstatus"
	"backend/internal/testutil"
	"backend/migrations"
)

// Runs the Goose migration and the as-of query against Postgres, where the
// CHECK constraints and TIMESTAMPTZ comparisons differ from SQLite.
func TestWriteAndAsOf_Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	scopedDSN := testutil.NewPGSchema(t, dsn, "playerstatus_test")
	require.NoError(t, dbmigrate.Run(scopedDSN, migrations.FS, "up", nil))
	db := testutil.OpenGORM(t, scopedDSN)
	ctx := context.Background()

	bulkAt := time.Date(2026, 12, 12, 8, 0, 0, 0, time.UTC)
	projAt := time.Date(2026, 12, 13, 17, 0, 0, 0, time.UTC)
	_, err := playerstatus.Write(ctx, db,
		playerstatus.Capture{ID: "bulk-1", Source: playerstatus.SourcePlayersBulk, Season: "2026", Week: 15, ObservedAt: bulkAt},
		[]playerstatus.Observation{{SleeperPlayerID: "4866", NflTeam: str("PHI"), InjuryStatus: str("Questionable")}}, nil, 36*time.Hour)
	require.NoError(t, err)
	_, err = playerstatus.Write(ctx, db,
		playerstatus.Capture{ID: "proj-1", Source: playerstatus.SourceProjections, Season: "2026", Week: 15, ObservedAt: projAt},
		[]playerstatus.Observation{
			{SleeperPlayerID: "4866", NflTeam: str("PHI"), InjuryStatus: str("Out")},
			{SleeperPlayerID: "8150", Applicability: playerstatus.Applicability{Week: 15}},
		}, nil, 0)
	require.NoError(t, err)

	got, err := playerstatus.AsOf(ctx, db, projAt.Add(-time.Hour), "", []string{"4866", "8150"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Questionable", *got["4866"].InjuryStatus)

	got, err = playerstatus.AsOf(ctx, db, projAt, "", []string{"4866", "8150"})
	require.NoError(t, err)
	require.Equal(t, "proj-1", got["4866"].CaptureID)
	require.Equal(t, "Out", *got["4866"].InjuryStatus)
	require.Nil(t, got["8150"].InjuryStatus)
	require.Equal(t, playerstatus.ApplicabilityWeek, got["8150"].Applicability)

	got, err = playerstatus.AsOf(ctx, db, projAt, playerstatus.SourcePlayersBulk, []string{"4866"})
	require.NoError(t, err)
	require.Equal(t, "bulk-1", got["4866"].CaptureID)

	// The CHECK constraint rejects game applicability without a game ID.
	err = db.Exec(`INSERT INTO sleeper_player_status_observations
		(capture_id, sleeper_player_id, source, observed_at, applicability)
		VALUES ('proj-1', 'x', 'sleeper_projections', now(), 'game')`).Error
	require.Error(t, err)
}

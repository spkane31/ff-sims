package playerstatus_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/models"
	"backend/internal/playerstatus"
	"backend/internal/sleeper"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.SleeperStatusCapture{}, &models.SleeperPlayerStatusObservation{}))
	return db
}

func str(s string) *string { return &s }

func write(t *testing.T, db *gorm.DB, c playerstatus.Capture, obs ...playerstatus.Observation) playerstatus.Report {
	t.Helper()
	r, err := playerstatus.Write(context.Background(), db, c, obs, nil, 36*time.Hour)
	require.NoError(t, err)
	return r
}

func TestAsOf_ReturnsLatestObservationAtOrBeforeTime(t *testing.T) {
	db := newTestDB(t)
	mon := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	tue := mon.Add(24 * time.Hour)
	write(t, db, playerstatus.Capture{ID: "bulk-mon", Source: playerstatus.SourcePlayersBulk, Season: "2026", Week: 12, ObservedAt: mon},
		playerstatus.Observation{SleeperPlayerID: "4017", NflTeam: str("BUF"), InjuryStatus: str("Questionable")})
	write(t, db, playerstatus.Capture{ID: "bulk-tue", Source: playerstatus.SourcePlayersBulk, Season: "2026", Week: 13, ObservedAt: tue},
		playerstatus.Observation{SleeperPlayerID: "4017", NflTeam: str("BUF"), InjuryStatus: str("Out")})

	ctx := context.Background()
	got, err := playerstatus.AsOf(ctx, db, mon.Add(time.Hour), "", []string{"4017", "9999"})
	require.NoError(t, err)
	require.Len(t, got, 1, "unobserved players are absent")
	require.Equal(t, "bulk-mon", got["4017"].CaptureID)
	require.Equal(t, "Questionable", *got["4017"].InjuryStatus)

	got, err = playerstatus.AsOf(ctx, db, tue, playerstatus.SourcePlayersBulk, []string{"4017"})
	require.NoError(t, err)
	require.Equal(t, "bulk-tue", got["4017"].CaptureID)
	require.Equal(t, 13, *got["4017"].ObservedWeek)

	got, err = playerstatus.AsOf(ctx, db, mon.Add(-time.Minute), "", []string{"4017"})
	require.NoError(t, err)
	require.Empty(t, got, "nothing was known before the first capture")

	got, err = playerstatus.AsOf(ctx, db, tue, playerstatus.SourceProjections, []string{"4017"})
	require.NoError(t, err)
	require.Empty(t, got, "source filter excludes bulk captures")
}

// A projection row whose nested player is tagged Out still carries a positive
// forecast for a later week. The observation records the current tag with
// unknown applicability, so it does not claim the player misses that week.
// The second row is metadata-only (no stats, null injury_status) and must be
// stored without implying healthy. The third has no nested player and is
// reported as missing coverage.
const projectionResponse = `[
  {"player_id":"4866","company":"rotowire","week":15,"season":"2026","game_id":"202615001",
   "stats":{"pts_ppr":14.2,"rush_yd":61},
   "player":{"position":"RB","team":"PHI","injury_status":"Out","injury_start_date":"2026-11-20","news_updated":1764000000000}},
  {"player_id":"4866","company":"sportradar","week":15,"season":"2026",
   "stats":{"pts_ppr":12.0},
   "player":{"position":"RB","team":"PHI","injury_status":"Out","injury_start_date":"2026-11-20","news_updated":1764000000000}},
  {"player_id":"8150","company":"rotowire","week":15,"season":"2026","stats":{},
   "player":{"position":"WR","team":null,"injury_status":null}},
  {"player_id":"7777","company":"rotowire","week":15,"season":"2026","stats":{"pts_ppr":3.1}},
  {"player_id":"1001","company":"rotowire","week":15,"season":"2026","stats":{},
   "player":{"position":"LB","team":"DAL","injury_status":"IR"}}
]`

func TestWrite_ProjectionCaptureSharesIDAndKeepsStatusSemantics(t *testing.T) {
	db := newTestDB(t)
	var rows []sleeper.ProjectionRow
	require.NoError(t, json.Unmarshal([]byte(projectionResponse), &rows))

	obs, missing := playerstatus.FromProjectionRows(rows)
	observedAt := time.Date(2026, 12, 13, 17, 0, 0, 0, time.UTC)
	c := playerstatus.Capture{ID: "proj-batch-42", Source: playerstatus.SourceProjections, Season: "2026", Week: 15, ObservedAt: observedAt}

	ctx := context.Background()
	r, err := playerstatus.Write(ctx, db, c, obs, missing, 0)
	require.NoError(t, err)
	require.Equal(t, 2, r.Observations, "duplicate company rows collapse, non-fantasy positions skipped")
	require.Equal(t, 2, r.ApplicabilityUnknown)
	require.Equal(t, 0, r.ConflictingDuplicates)
	require.Equal(t, 1, r.MissingMetadata)
	require.Equal(t, []string{"7777"}, r.MissingMetadataPlayerIDs)

	// Same-run retry is a no-op.
	_, err = playerstatus.Write(ctx, db, c, obs, missing, 0)
	require.NoError(t, err)

	var stored []models.SleeperPlayerStatusObservation
	require.NoError(t, db.Order("sleeper_player_id").Find(&stored).Error)
	require.Len(t, stored, 2)
	for _, s := range stored {
		require.Equal(t, "proj-batch-42", s.CaptureID)
		require.True(t, observedAt.Equal(s.ObservedAt))
		require.Equal(t, playerstatus.ApplicabilityUnknown, s.Applicability)
		require.Nil(t, s.ApplicableGameID, "a projection game_id is not evidence for the injury tag")
	}

	out := stored[0]
	require.Equal(t, "4866", out.SleeperPlayerID)
	require.Equal(t, "Out", *out.InjuryStatus)
	require.Equal(t, "PHI", *out.NflTeam)
	require.Equal(t, "2026-11-20", *out.InjuryStartDate)
	require.Equal(t, int64(1764000000000), *out.NewsUpdatedMs)

	metaOnly := stored[1]
	require.Equal(t, "8150", metaOnly.SleeperPlayerID)
	require.Nil(t, metaOnly.InjuryStatus, "null status must not be stored as healthy")
	require.Nil(t, metaOnly.NflTeam)

	var captures []models.SleeperStatusCapture
	require.NoError(t, db.Find(&captures).Error)
	require.Len(t, captures, 1)
	require.Equal(t, 2, captures[0].ObservationCount)
}

func TestWrite_RecordsVerifiedApplicabilityOnly(t *testing.T) {
	db := newTestDB(t)
	at := time.Date(2026, 12, 13, 17, 0, 0, 0, time.UTC)
	r := write(t, db, playerstatus.Capture{ID: "c1", Source: playerstatus.SourceProjections, ObservedAt: at},
		playerstatus.Observation{SleeperPlayerID: "1", InjuryStatus: str("Out"), Applicability: playerstatus.Applicability{GameID: "g1"}},
		playerstatus.Observation{SleeperPlayerID: "2", InjuryStatus: str("Doubtful"), Applicability: playerstatus.Applicability{Week: 15}},
		playerstatus.Observation{SleeperPlayerID: "3", InjuryStatus: str("IR")},
	)
	require.Equal(t, 1, r.ApplicabilityUnknown)

	got, err := playerstatus.AsOf(context.Background(), db, at, "", []string{"1", "2", "3"})
	require.NoError(t, err)
	require.Equal(t, playerstatus.ApplicabilityGame, got["1"].Applicability)
	require.Equal(t, "g1", *got["1"].ApplicableGameID)
	require.Equal(t, playerstatus.ApplicabilityWeek, got["2"].Applicability)
	require.Equal(t, 15, *got["2"].ApplicableWeek)
	require.Equal(t, playerstatus.ApplicabilityUnknown, got["3"].Applicability)

	_, err = playerstatus.Write(context.Background(), db,
		playerstatus.Capture{ID: "c2", Source: playerstatus.SourceProjections, ObservedAt: at},
		[]playerstatus.Observation{{SleeperPlayerID: "1", Applicability: playerstatus.Applicability{GameID: "g1", Week: 15}}}, nil, 0)
	require.Error(t, err)
}

func TestWrite_ReportsStalePreviousCapture(t *testing.T) {
	db := newTestDB(t)
	first := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	r := write(t, db, playerstatus.Capture{ID: "a", Source: playerstatus.SourcePlayersBulk, ObservedAt: first})
	require.Nil(t, r.PreviousCaptureAt)
	require.False(t, r.Stale)

	r = write(t, db, playerstatus.Capture{ID: "b", Source: playerstatus.SourcePlayersBulk, ObservedAt: first.Add(24 * time.Hour)})
	require.True(t, first.Equal(*r.PreviousCaptureAt))
	require.False(t, r.Stale)

	r = write(t, db, playerstatus.Capture{ID: "c", Source: playerstatus.SourcePlayersBulk, ObservedAt: first.Add(96 * time.Hour)})
	require.True(t, r.Stale, "a three-day gap after a daily capture is missing coverage")
}

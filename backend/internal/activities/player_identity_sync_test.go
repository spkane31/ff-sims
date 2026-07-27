package activities_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"go.temporal.io/sdk/testsuite"
	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
)

// runSyncPlayerIdentities runs SyncPlayerIdentities through Temporal's
// activity test harness rather than calling it directly with
// context.Background() — the activity calls activity.RecordHeartbeat, which
// panics outside a real activity context. Conflicts are reported as data in
// the returned result (Conflicts/ConflictDetails), not as an error — a
// non-nil error here means a genuine unexpected failure, so on error the
// result is the zero value (Temporal doesn't surface a value alongside an
// activity error).
func runSyncPlayerIdentities(t *testing.T, a *activities.PlayerSyncActivities) (activities.PlayerIdentitySyncResult, error) {
	t.Helper()
	ts := testsuite.WorkflowTestSuite{}
	env := ts.NewTestActivityEnvironment()
	env.RegisterActivity(a.SyncPlayerIdentities)
	val, err := env.ExecuteActivity(a.SyncPlayerIdentities)
	if err != nil {
		return activities.PlayerIdentitySyncResult{}, err
	}
	var res activities.PlayerIdentitySyncResult
	if decodeErr := val.Get(&res); decodeErr != nil {
		t.Fatalf("decode result: %v", decodeErr)
	}
	return res, nil
}

func seedSleeperPlayer(t *testing.T, db *gorm.DB, id, espnID, name, position, team string) {
	t.Helper()
	if err := db.Create(&models.SleeperPlayer{
		SleeperPlayerID: id,
		EspnID:          espnID,
		FullName:        name,
		Position:        position,
		NflTeam:         team,
	}).Error; err != nil {
		t.Fatalf("seed sleeper player %s: %v", id, err)
	}
}

func seedPlayer(t *testing.T, db *gorm.DB, espnID int64, sleeperID, name, position, team string) models.Player {
	t.Helper()
	p := models.Player{ESPNID: espnID, SleeperID: sleeperID, Name: name, Position: position, Team: team}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("seed player espn_id=%d: %v", espnID, err)
	}
	return p
}

// A production run once failed with a Temporal heartbeat timeout because
// the activity only heartbeated once per up-to-500-row batch, and each row
// is its own DB round-trip. This seeds more rows than one heartbeat
// interval to exercise that inner-loop heartbeat path (see
// playerIdentityHeartbeatInterval) and confirm it doesn't affect
// correctness — a true regression test for the timeout itself would need a
// live heartbeat-timeout clock, which isn't practical here (the SDK's test
// harness documents that its heartbeat listener is internally throttled and
// won't reflect every call).
func TestSyncPlayerIdentities_HeartbeatsAcrossMultipleIntervalsWithinOneBatch(t *testing.T) {
	db := newTestDB(t)
	const rowCount = 60 // > 2x playerIdentityHeartbeatInterval (25), still one batch (< 500)
	for i := 0; i < rowCount; i++ {
		id := "hb" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		seedSleeperPlayer(t, db, id, "", "Player "+id, "WR", "SF")
	}

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Created != rowCount {
		t.Errorf("expected all %d rows created, got %+v", rowCount, result)
	}

	var count int64
	db.Model(&models.Player{}).Count(&count)
	if count != rowCount {
		t.Errorf("expected %d players rows, got %d", rowCount, count)
	}
}

// syncIdentityBatch now runs each batch's writes in a single DB transaction
// (see its doc comment) instead of letting each row auto-commit
// individually. This exercises all three write paths — create, link, and a
// DEF link (which, like everything else, now matches by espn_id — team
// defenses use small stable negative ESPN IDs) — together in one
// batch/transaction to catch any spot that missed threading the shared tx
// through (e.g. a stray a.DB call bypassing it), which unit tests written
// before the refactor wouldn't have caught since they each only exercised
// one path per batch.
func TestSyncPlayerIdentities_MixedBatchAllPathsCommitTogether(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 555, "", "ESPN Linked WR", "RB", "DAL")
	seedPlayer(t, db, -16012, "", "Chiefs", "DEF", "")
	seedSleeperPlayer(t, db, "create-me", "9999", "New Guy", "WR", "SF")
	seedSleeperPlayer(t, db, "link-me", "555", "ESPN Linked WR", "RB", "DAL")
	seedSleeperPlayer(t, db, "KC", "-16012", "Chiefs", "DEF", "KC")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Created != 1 || result.Linked != 2 {
		t.Errorf("expected 1 created + 2 linked (skill + DEF), got %+v", result)
	}

	var count int64
	db.Model(&models.Player{}).Count(&count)
	if count != 3 {
		t.Errorf("expected 3 players rows total, got %d", count)
	}
	for _, id := range []string{"create-me", "link-me", "KC"} {
		var p models.Player
		if err := db.Where("sleeper_id = ?", id).First(&p).Error; err != nil {
			t.Errorf("expected sleeper_id %s to be linked/created: %v", id, err)
		}
	}
}

func TestSyncPlayerIdentities_CreatesRowForUnmatchedEspnID(t *testing.T) {
	db := newTestDB(t)
	seedSleeperPlayer(t, db, "sp1", "9999", "New Guy", "WR", "SF")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Created != 1 || result.Linked != 0 || result.Conflicts != 0 {
		t.Errorf("expected 1 created, got %+v", result)
	}

	var p models.Player
	if err := db.Where("sleeper_id = ?", "sp1").First(&p).Error; err != nil {
		t.Fatalf("expected a players row for sp1: %v", err)
	}
	if p.ESPNID != 9999 || p.Name != "New Guy" || p.Position != "WR" || p.Team != "SF" {
		t.Errorf("unexpected created player: %+v", p)
	}
}

func TestSyncPlayerIdentities_LinksExistingESPNRowWithoutClobberingItsFields(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 555, "", "ESPN Name", "RB", "DAL")
	seedSleeperPlayer(t, db, "sp2", "555", "Sleeper Name (stale)", "RB", "DAL")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Linked != 1 || result.Created != 0 {
		t.Errorf("expected 1 linked, got %+v", result)
	}

	var p models.Player
	if err := db.Where("espn_id = ?", 555).First(&p).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if p.SleeperID != "sp2" {
		t.Errorf("expected sleeper_id sp2, got %q", p.SleeperID)
	}
	// ESPN stays authoritative — Sleeper's name must not overwrite it.
	if p.Name != "ESPN Name" {
		t.Errorf("expected Name to remain ESPN-sourced, got %q", p.Name)
	}
}

func TestSyncPlayerIdentities_IdempotentRerun(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 1, "", "Existing", "QB", "KC")
	seedSleeperPlayer(t, db, "sp3", "1", "Existing", "QB", "KC")
	seedSleeperPlayer(t, db, "sp4", "", "No ESPN Mapping", "TE", "MIA")

	a := &activities.PlayerSyncActivities{DB: db}
	if _, err := runSyncPlayerIdentities(t, a); err != nil {
		t.Fatalf("first run error: %v", err)
	}

	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result.Linked != 0 || result.Created != 0 {
		t.Errorf("expected a re-run to be a no-op, got %+v", result)
	}

	var count int64
	db.Model(&models.Player{}).Count(&count)
	if count != 2 {
		t.Errorf("expected exactly 2 players rows after two runs, got %d", count)
	}
}

func TestSyncPlayerIdentities_EmptyOrZeroEspnIDTreatedAsUnset(t *testing.T) {
	db := newTestDB(t)
	seedSleeperPlayer(t, db, "sp5", "", "No Mapping", "K", "NYJ")
	seedSleeperPlayer(t, db, "sp6", "0", "Zero Mapping", "K", "BUF")
	seedSleeperPlayer(t, db, "sp7", "not-a-number", "Garbage Mapping", "K", "MIA")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Created != 3 {
		t.Errorf("expected all 3 to be created (benign non-matches), got %+v", result)
	}
	for _, id := range []string{"sp5", "sp6", "sp7"} {
		var p models.Player
		if err := db.Where("sleeper_id = ?", id).First(&p).Error; err != nil {
			t.Fatalf("expected a players row for %s: %v", id, err)
		}
		if p.ESPNID != 0 {
			t.Errorf("%s: expected espn_id 0 (unset sentinel), got %d", id, p.ESPNID)
		}
	}
}

// Conflicts are reported in the result, not as an activity/workflow error —
// see SyncPlayerIdentities' doc comment for why (a handful of these can be
// permanently unresolvable Sleeper duplicate-ID rows, so failing every run
// forever isn't useful; PlayerDatabaseSyncWorkflow carries them into
// PlayerSyncReport.IdentityConflictDetails specifically so they're easy to
// find without digging through activity failure history).
func TestSyncPlayerIdentities_ConflictWhenEspnMatchAlreadyClaimedByAnotherSleeperID(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 42, "already-linked", "Existing", "WR", "GB")
	seedSleeperPlayer(t, db, "different-sleeper-id", "42", "Existing", "WR", "GB")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Conflicts != 1 || len(result.ConflictDetails) != 1 {
		t.Fatalf("expected 1 conflict reported in the result, got %+v", result)
	}
	if !containsAll(result.ConflictDetails[0], "different-sleeper-id", "espn_id=42", "already linked to a different sleeper_id") {
		t.Errorf("conflict detail not itemized as expected: %v", result.ConflictDetails[0])
	}

	// The claim must not have been overwritten by the conflicting attempt.
	var p models.Player
	db.Where("espn_id = ?", 42).First(&p)
	if p.SleeperID != "already-linked" {
		t.Errorf("expected existing claim to survive the conflict, got sleeper_id=%q", p.SleeperID)
	}
}

// Team defenses use small stable negative ESPN "player" IDs (e.g. -16025
// for the 49ers) — a production run confirmed real data has these — so they
// match by espn_id exactly like every other position, no special-casing.
func TestSyncPlayerIdentities_DEF_LinksExistingRowByNegativeEspnID(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, -16025, "", "49ers D/ST", "D/ST", "")
	seedSleeperPlayer(t, db, "SF", "-16025", "49ers", "DEF", "SF")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Linked != 1 {
		t.Errorf("expected the DEF row to link by espn_id, got %+v", result)
	}

	var p models.Player
	if err := db.Where("espn_id = ?", -16025).First(&p).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if p.SleeperID != "SF" {
		t.Errorf("expected sleeper_id SF, got %q", p.SleeperID)
	}
}

func TestSyncPlayerIdentities_DEF_CreatesRowForUnmatchedNegativeEspnID(t *testing.T) {
	db := newTestDB(t)
	// No existing players row for this DEF at all.
	seedSleeperPlayer(t, db, "KC", "-16012", "Chiefs", "DEF", "KC")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("expected the DEF row to be created, got %+v", result)
	}

	var p models.Player
	if err := db.Where("sleeper_id = ?", "KC").First(&p).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if p.ESPNID != -16012 {
		t.Errorf("expected the negative espn_id to be preserved on create, got %d", p.ESPNID)
	}
}

// Sleeper labels stale duplicate entries with the literal name "Duplicate
// Player" — a production run found several, sometimes with the junk entry's
// sleeper_player_id sorting before the real player's (ascending-ID batch
// order), wrongly winning the espn_id claim and permanently blocking the
// real player from ever linking. These rows must be skipped entirely: never
// matched against, never used to create a players row.
func TestSyncPlayerIdentities_ExcludesDuplicatePlayerMarker(t *testing.T) {
	db := newTestDB(t)
	// "junk" sorts before "real" alphabetically/ascending, matching the
	// production case where the placeholder processed first.
	seedSleeperPlayer(t, db, "junk", "777", "Duplicate Player", "CB", "")
	seedSleeperPlayer(t, db, "real", "777", "Jayrone Elliott", "LB", "")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Conflicts != 0 {
		t.Errorf("expected no conflict — the placeholder row should never have competed for espn_id=777, got %+v", result)
	}
	if result.Created != 1 {
		t.Errorf("expected exactly 1 row created (the real player), got %+v", result)
	}

	var count int64
	db.Model(&models.Player{}).Where("sleeper_id = ?", "junk").Count(&count)
	if count != 0 {
		t.Error("the Duplicate Player row must never create a players row")
	}
	var p models.Player
	if err := db.Where("sleeper_id = ?", "real").First(&p).Error; err != nil {
		t.Fatalf("expected the real player to link/create successfully: %v", err)
	}
	if p.ESPNID != 777 {
		t.Errorf("expected espn_id 777, got %d", p.ESPNID)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestIsUniqueViolation(t *testing.T) {
	if activities.IsUniqueViolation(errors.New("some other error")) {
		t.Error("plain error should not be classified as a unique violation")
	}
	pgErr := &pgconn.PgError{Code: "23505"}
	if !activities.IsUniqueViolation(pgErr) {
		t.Error("expected a wrapped 23505 PgError to be classified as a unique violation")
	}
	other := &pgconn.PgError{Code: "23503"}
	if activities.IsUniqueViolation(other) {
		t.Error("a different SQLSTATE must not be classified as a unique violation")
	}
}

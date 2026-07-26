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
// panics outside a real activity context. On error, the result is the
// zero value (Temporal doesn't surface a value alongside an activity
// error), so conflict-path tests must assert on the error message, not on
// counts in the returned result.
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

func TestSyncPlayerIdentities_ConflictWhenEspnMatchAlreadyClaimedByAnotherSleeperID(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 42, "already-linked", "Existing", "WR", "GB")
	seedSleeperPlayer(t, db, "different-sleeper-id", "42", "Existing", "WR", "GB")

	a := &activities.PlayerSyncActivities{DB: db}
	_, err := runSyncPlayerIdentities(t, a)
	if err == nil {
		t.Fatal("expected a conflict error")
	}
	if !containsAll(err.Error(), "different-sleeper-id", "espn_id=42", "already linked to a different sleeper_id") {
		t.Errorf("error message not itemized as expected: %v", err)
	}

	// The claim must not have been overwritten by the conflicting attempt.
	var p models.Player
	db.Where("espn_id = ?", 42).First(&p)
	if p.SleeperID != "already-linked" {
		t.Errorf("expected existing claim to survive the conflict, got sleeper_id=%q", p.SleeperID)
	}
}

func TestSyncPlayerIdentities_DEF_LinksByTeamAbbreviation(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 0, "", "San Francisco", "D/ST", "SF")
	seedSleeperPlayer(t, db, "SF", "", "49ers", "DEF", "SF")

	a := &activities.PlayerSyncActivities{DB: db}
	result, err := runSyncPlayerIdentities(t, a)
	if err != nil {
		t.Fatalf("SyncPlayerIdentities error: %v", err)
	}
	if result.Linked != 1 {
		t.Errorf("expected the DEF row to link by team abbreviation, got %+v", result)
	}

	var p models.Player
	if err := db.Where("team = ? AND position = ?", "SF", "D/ST").First(&p).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if p.SleeperID != "SF" {
		t.Errorf("expected sleeper_id SF, got %q", p.SleeperID)
	}
}

func TestSyncPlayerIdentities_DEF_ConflictWhenNoTeamMatch(t *testing.T) {
	db := newTestDB(t)
	// No existing "KC" D/ST row in players at all.
	seedSleeperPlayer(t, db, "KC", "", "Chiefs", "DEF", "KC")

	a := &activities.PlayerSyncActivities{DB: db}
	_, err := runSyncPlayerIdentities(t, a)
	if err == nil {
		t.Fatal("expected a conflict error for an unresolvable DEF team")
	}
	if !containsAll(err.Error(), `team="KC"`, "resolved to 0 players rows") {
		t.Errorf("error message not itemized as expected: %v", err)
	}
}

func TestSyncPlayerIdentities_DEF_ConflictOnDuplicateTeamRows(t *testing.T) {
	db := newTestDB(t)
	seedPlayer(t, db, 100, "", "Dup A", "DEF", "NE")
	seedPlayer(t, db, 101, "", "Dup B", "D/ST", "NE")
	seedSleeperPlayer(t, db, "NE", "", "Patriots", "DEF", "NE")

	a := &activities.PlayerSyncActivities{DB: db}
	_, err := runSyncPlayerIdentities(t, a)
	if err == nil {
		t.Fatal("expected a conflict error for a team resolving to more than one players row")
	}
	if !containsAll(err.Error(), `team="NE"`, "resolved to 2 players rows") {
		t.Errorf("error message not itemized as expected: %v", err)
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

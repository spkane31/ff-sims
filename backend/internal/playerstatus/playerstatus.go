// Package playerstatus stores timestamped snapshots of Sleeper player
// availability (NFL team, roster status, injury designation) so replay can ask
// what was known about a player at a given time.
//
// Sleeper reports a player's current injury tag, not a designation for a
// specific game, so observations default to unknown applicability. A current
// Out or IR observation can therefore coexist with a positive forecast for a
// later week.
package playerstatus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

// Sources identify which Sleeper response a capture was taken from.
const (
	SourcePlayersBulk = "sleeper_players_bulk"
	SourceProjections = "sleeper_projections"
)

// Applicability values stored with each observation.
const (
	ApplicabilityUnknown = "unknown"
	ApplicabilityGame    = "game"
	ApplicabilityWeek    = "week"
)

// FantasyPositions limits snapshots to the positions captured for projections.
var FantasyPositions = map[string]bool{
	"QB": true, "RB": true, "WR": true, "TE": true, "K": true, "DEF": true,
}

// maxReportedIDs caps player ID lists carried in reports so a bad response
// cannot bloat Temporal workflow results.
const maxReportedIDs = 50

// Capture identifies one fetch. ID is shared with the projection batch when
// the observations come from a projection response.
type Capture struct {
	ID         string
	Source     string
	Season     string // empty when unknown
	Week       int    // 0 when unknown
	ObservedAt time.Time
}

// Applicability ties a designation to a game or target week. Set it only when
// verified evidence supports it; the zero value records unknown.
type Applicability struct {
	GameID string
	Week   int
}

// Observation is one player's availability as reported in a capture.
type Observation struct {
	SleeperPlayerID string
	NflTeam         *string
	RosterStatus    *string
	InjuryStatus    *string
	InjuryStartDate *string
	NewsUpdatedMs   *int64
	Applicability   Applicability
}

// Report summarizes one status write for Temporal workflow results and logs.
type Report struct {
	CaptureID            string
	Source               string
	ObservedAt           time.Time
	Observations         int
	ApplicabilityUnknown int

	// MissingMetadata counts projection rows without a nested player object,
	// so their status was not observed. MissingMetadataPlayerIDs holds up to
	// maxReportedIDs of them.
	MissingMetadata          int
	MissingMetadataPlayerIDs []string

	// ConflictingDuplicates counts players that appeared more than once in the
	// response with different status values. The first row wins.
	ConflictingDuplicates int

	// PreviousCaptureAt is the latest earlier capture from the same source, or
	// nil if none exists. Stale is true when it is older than the expected
	// cadence, meaning coverage between the two captures is missing.
	PreviousCaptureAt *time.Time
	Stale             bool

	// Warnings lists non-fatal gaps such as an unknown observed week.
	Warnings []string

	// WriteError is set when the status write failed. Callers report it rather
	// than failing, since retrying would refetch Sleeper's bulk feed.
	WriteError string
}

// FromBulkPlayers converts the bulk /v1/players/nfl response into
// observations for fantasy-position players who are active, on a team, or
// carry an injury designation. Retired players without any of those are
// skipped to keep daily snapshots small.
func FromBulkPlayers(players map[string]sleeper.Player) []Observation {
	obs := make([]Observation, 0, len(players)/4)
	for id, p := range players {
		if !FantasyPositions[p.Position] {
			continue
		}
		active := p.Active != nil && *p.Active
		if !active && p.Team == "" && p.InjuryStatus == nil {
			continue
		}
		o := fromAvailability(id, p.PlayerAvailability)
		if p.Team != "" {
			team := p.Team
			o.NflTeam = &team
		}
		obs = append(obs, o)
	}
	return obs
}

// FromProjectionRows converts the nested player objects of a projection
// response into observations, including metadata-only rows. Rows without a
// nested player are counted as missing coverage. Rows outside fantasy
// positions are skipped.
func FromProjectionRows(rows []sleeper.ProjectionRow) (obs []Observation, missing []string) {
	for _, r := range rows {
		if r.PlayerID == "" {
			continue
		}
		if r.Player == nil {
			missing = append(missing, r.PlayerID)
			continue
		}
		if r.Player.Position != "" && !FantasyPositions[r.Player.Position] {
			continue
		}
		o := fromAvailability(r.PlayerID, r.Player.PlayerAvailability)
		if r.Player.Team != nil && *r.Player.Team != "" {
			team := *r.Player.Team
			o.NflTeam = &team
		}
		obs = append(obs, o)
	}
	return obs, missing
}

func fromAvailability(id string, a sleeper.PlayerAvailability) Observation {
	o := Observation{
		SleeperPlayerID: id,
		RosterStatus:    nonEmpty(a.Status),
		InjuryStatus:    nonEmpty(a.InjuryStatus),
		InjuryStartDate: nonEmpty(a.InjuryStartDate),
	}
	if a.NewsUpdated != nil && a.NewsUpdated.Valid {
		v := a.NewsUpdated.Value
		o.NewsUpdatedMs = &v
	}
	return o
}

func nonEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	v := *s
	return &v
}

// Write stores the capture and its observations in one transaction. Pass the
// projection writer's transaction as db to publish both streams atomically.
// Re-running with the same capture ID is a no-op, which makes same-run
// retries idempotent. missing lists projection rows without player metadata.
// staleAfter is the expected gap between captures from this source.
func Write(ctx context.Context, db *gorm.DB, c Capture, obs []Observation, missing []string, staleAfter time.Duration) (Report, error) {
	r := Report{
		CaptureID:       c.ID,
		Source:          c.Source,
		ObservedAt:      c.ObservedAt,
		MissingMetadata: len(missing),
	}
	if len(missing) > 0 {
		r.MissingMetadataPlayerIDs = capIDs(missing)
	}
	if c.ID == "" || c.Source == "" || c.ObservedAt.IsZero() {
		return r, errors.New("playerstatus: capture ID, source and observed time are required")
	}
	observedAt := c.ObservedAt.UTC()

	rows, conflicts, err := toRows(c, observedAt, obs)
	if err != nil {
		return r, err
	}
	r.ConflictingDuplicates = conflicts
	for _, row := range rows {
		if row.Applicability == ApplicabilityUnknown {
			r.ApplicabilityUnknown++
		}
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prev models.SleeperStatusCapture
		res := tx.Where("source = ? AND observed_at < ?", c.Source, observedAt).
			Order("observed_at DESC").Limit(1).Find(&prev)
		if res.Error != nil {
			return fmt.Errorf("find previous %s capture: %w", c.Source, res.Error)
		}
		if res.RowsAffected > 0 {
			at := prev.ObservedAt.UTC()
			r.PreviousCaptureAt = &at
			r.Stale = staleAfter > 0 && observedAt.Sub(at) > staleAfter
		}

		capture := models.SleeperStatusCapture{
			CaptureID:        c.ID,
			Source:           c.Source,
			ObservedSeason:   optString(c.Season),
			ObservedWeek:     optInt(c.Week),
			ObservedAt:       observedAt,
			ObservationCount: len(rows),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&capture).Error; err != nil {
			return fmt.Errorf("insert capture %s: %w", c.ID, err)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 500).Error; err != nil {
			return fmt.Errorf("insert %d observations for capture %s: %w", len(rows), c.ID, err)
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	r.Observations = len(rows)
	return r, nil
}

func toRows(c Capture, observedAt time.Time, obs []Observation) ([]models.SleeperPlayerStatusObservation, int, error) {
	byID := make(map[string]int, len(obs))
	rows := make([]models.SleeperPlayerStatusObservation, 0, len(obs))
	conflicts := 0
	for _, o := range obs {
		if o.SleeperPlayerID == "" {
			continue
		}
		row := models.SleeperPlayerStatusObservation{
			CaptureID:       c.ID,
			SleeperPlayerID: o.SleeperPlayerID,
			Source:          c.Source,
			ObservedSeason:  optString(c.Season),
			ObservedWeek:    optInt(c.Week),
			ObservedAt:      observedAt,
			NflTeam:         o.NflTeam,
			RosterStatus:    o.RosterStatus,
			InjuryStatus:    o.InjuryStatus,
			InjuryStartDate: o.InjuryStartDate,
			NewsUpdatedMs:   o.NewsUpdatedMs,
			Applicability:   ApplicabilityUnknown,
		}
		switch {
		case o.Applicability.GameID != "" && o.Applicability.Week != 0:
			return nil, 0, fmt.Errorf("playerstatus: player %s has both game and week applicability", o.SleeperPlayerID)
		case o.Applicability.GameID != "":
			row.Applicability = ApplicabilityGame
			row.ApplicableGameID = optString(o.Applicability.GameID)
		case o.Applicability.Week != 0:
			row.Applicability = ApplicabilityWeek
			row.ApplicableWeek = optInt(o.Applicability.Week)
		}
		if i, ok := byID[o.SleeperPlayerID]; ok {
			if !sameStatus(rows[i], row) {
				conflicts++
			}
			continue
		}
		byID[o.SleeperPlayerID] = len(rows)
		rows = append(rows, row)
	}
	return rows, conflicts, nil
}

func sameStatus(a, b models.SleeperPlayerStatusObservation) bool {
	return eqStr(a.NflTeam, b.NflTeam) && eqStr(a.RosterStatus, b.RosterStatus) &&
		eqStr(a.InjuryStatus, b.InjuryStatus) && eqStr(a.InjuryStartDate, b.InjuryStartDate) &&
		eqInt64(a.NewsUpdatedMs, b.NewsUpdatedMs)
}

func eqStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func capIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	if len(out) > maxReportedIDs {
		out = out[:maxReportedIDs]
	}
	return out
}

// AsOf returns the latest observation at or before asOf for each requested
// player. source restricts the lookup to one capture source; empty means any.
// Players never observed by asOf are absent from the result.
func AsOf(ctx context.Context, db *gorm.DB, asOf time.Time, source string, playerIDs []string) (map[string]models.SleeperPlayerStatusObservation, error) {
	out := make(map[string]models.SleeperPlayerStatusObservation, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	asOf = asOf.UTC()

	latest := db.Model(&models.SleeperPlayerStatusObservation{}).
		Select("sleeper_player_id, MAX(observed_at) AS observed_at").
		Where("sleeper_player_id IN ? AND observed_at <= ?", playerIDs, asOf).
		Group("sleeper_player_id")
	if source != "" {
		latest = latest.Where("source = ?", source)
	}

	q := db.WithContext(ctx).Table("sleeper_player_status_observations AS o").
		Select("o.*").
		Joins("JOIN (?) AS l ON l.sleeper_player_id = o.sleeper_player_id AND l.observed_at = o.observed_at", latest).
		Order("o.source")
	if source != "" {
		q = q.Where("o.source = ?", source)
	}

	var rows []models.SleeperPlayerStatusObservation
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("player status as of %s: %w", asOf.Format(time.RFC3339), err)
	}
	for _, row := range rows {
		if _, seen := out[row.SleeperPlayerID]; !seen {
			out[row.SleeperPlayerID] = row
		}
	}
	return out, nil
}

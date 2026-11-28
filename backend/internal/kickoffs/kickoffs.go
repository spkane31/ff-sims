// Package kickoffs loads operator-supplied NFL kickoff times into
// nfl_kickoffs, which projection captures use to decide replay eligibility.
package kickoffs

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"backend/internal/models"
)

// Header is the expected CSV header. kickoff is RFC 3339 with an offset
// (e.g. 2026-12-06T13:00:00-05:00); status is "confirmed" for times checked
// against the official NFL schedule or "provisional" otherwise.
var Header = []string{"season", "week", "away", "home", "kickoff", "status"}

type weekKey struct {
	season string
	week   int
}

// ParseCSV reads one game per line and returns a kickoff row for each team.
func ParseCSV(r io.Reader, source string) ([]models.NFLKickoff, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if strings.Join(header, ",") != strings.Join(Header, ",") {
		return nil, fmt.Errorf("header %q, want %q", strings.Join(header, ","), strings.Join(Header, ","))
	}
	var rows []models.NFLKickoff
	seen := map[string]int{}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		week, err := strconv.Atoi(rec[1])
		if err != nil || week < 1 || week > 18 {
			return nil, fmt.Errorf("line %d: week %q must be 1-18", line, rec[1])
		}
		kickoff, err := time.Parse(time.RFC3339, rec[4])
		if err != nil {
			return nil, fmt.Errorf("line %d: kickoff %q: %w", line, rec[4], err)
		}
		var verified bool
		switch rec[5] {
		case "confirmed":
			verified = true
		case "provisional":
		default:
			return nil, fmt.Errorf("line %d: status %q must be confirmed or provisional", line, rec[5])
		}
		season, away, home := rec[0], strings.ToUpper(rec[2]), strings.ToUpper(rec[3])
		if season == "" || away == "" || home == "" || away == home {
			return nil, fmt.Errorf("line %d: season, away and home are required and teams must differ", line)
		}
		for _, team := range [][2]string{{away, home}, {home, away}} {
			k := fmt.Sprintf("%s/%d/%s", season, week, team[0])
			if prev, ok := seen[k]; ok {
				return nil, fmt.Errorf("line %d: %s already scheduled on line %d", line, k, prev)
			}
			seen[k] = line
			rows = append(rows, models.NFLKickoff{
				Season: season, Week: week, Team: team[0], Opponent: team[1],
				KickoffAt: kickoff.UTC(), Verified: verified, Source: source,
			})
		}
	}
	if len(rows) == 0 {
		return nil, errors.New("no games in file")
	}
	return rows, nil
}

// Replace swaps in rows for every season/week they cover, in one
// transaction, so a corrected file replaces moved or cancelled games.
func Replace(db *gorm.DB, rows []models.NFLKickoff) error {
	weeks := map[weekKey]bool{}
	for _, r := range rows {
		weeks[weekKey{r.Season, r.Week}] = true
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for w := range weeks {
			if err := tx.Where("season = ? AND week = ?", w.season, w.week).Delete(&models.NFLKickoff{}).Error; err != nil {
				return fmt.Errorf("clear %s week %d: %w", w.season, w.week, err)
			}
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
			return fmt.Errorf("insert kickoffs: %w", err)
		}
		return nil
	})
}

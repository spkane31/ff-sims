package kickoffs_test

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend/internal/kickoffs"
	"backend/internal/models"
)

func TestParseCSV(t *testing.T) {
	in := `season,week,away,home,kickoff,status
2026,13,gb,DET,2026-12-03T20:15:00-05:00,confirmed
2026,13,DAL,SEA,2026-12-06T16:25:00-05:00,provisional
`
	rows, err := kickoffs.ParseCSV(strings.NewReader(in), "nfl.com")
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("expected a row per team (4), got %d", len(rows))
	}
	gb := rows[0]
	if gb.Team != "GB" || gb.Opponent != "DET" || !gb.Verified || gb.Source != "nfl.com" ||
		!gb.KickoffAt.Equal(time.Date(2026, 12, 4, 1, 15, 0, 0, time.UTC)) || gb.KickoffAt.Location() != time.UTC {
		t.Errorf("unexpected row %+v", gb)
	}
	if rows[3].Team != "SEA" || rows[3].Verified {
		t.Errorf("provisional row %+v", rows[3])
	}
}

func TestParseCSV_Rejects(t *testing.T) {
	for name, in := range map[string]string{
		"bad header":    "season,week,team\n",
		"bad status":    "season,week,away,home,kickoff,status\n2026,13,GB,DET,2026-12-03T20:15:00-05:00,maybe\n",
		"no offset":     "season,week,away,home,kickoff,status\n2026,13,GB,DET,2026-12-03T20:15:00,confirmed\n",
		"double booked": "season,week,away,home,kickoff,status\n2026,13,GB,DET,2026-12-03T20:15:00-05:00,confirmed\n2026,13,GB,CHI,2026-12-06T13:00:00-05:00,confirmed\n",
		"week 19":       "season,week,away,home,kickoff,status\n2026,19,GB,DET,2026-12-03T20:15:00-05:00,confirmed\n",
		"empty":         "season,week,away,home,kickoff,status\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := kickoffs.ParseCSV(strings.NewReader(in), "x"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestReplace_SwapsCoveredWeeksOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.NFLKickoff{}); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 12, 6, 18, 0, 0, 0, time.UTC)
	db.Create(&models.NFLKickoff{Season: "2026", Week: 13, Team: "LV", Opponent: "LAC", KickoffAt: old, Verified: true, Source: "x"})
	db.Create(&models.NFLKickoff{Season: "2026", Week: 14, Team: "LV", Opponent: "DEN", KickoffAt: old.AddDate(0, 0, 7), Verified: true, Source: "x"})

	moved := old.Add(3*time.Hour + 25*time.Minute)
	err = kickoffs.Replace(db, []models.NFLKickoff{
		{Season: "2026", Week: 13, Team: "KC", Opponent: "DEN", KickoffAt: moved, Verified: true, Source: "y"},
		{Season: "2026", Week: 13, Team: "DEN", Opponent: "KC", KickoffAt: moved, Verified: true, Source: "y"},
	})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	var w13, w14 []models.NFLKickoff
	db.Where("week = 13").Order("team").Find(&w13)
	db.Where("week = 14").Find(&w14)
	if len(w13) != 2 || w13[0].Team != "DEN" || w13[1].Team != "KC" {
		t.Errorf("week 13 not replaced: %+v", w13)
	}
	if len(w14) != 1 {
		t.Errorf("week 14 should be untouched: %+v", w14)
	}
}

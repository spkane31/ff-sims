package handlers

import (
	"cmp"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"

	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type draftRewindReport struct {
	Year        uint              `json:"year"`
	ThroughWeek uint              `json:"through_week"`
	RankedPicks int               `json:"ranked_picks"`
	TotalPicks  int               `json:"total_picks"`
	Picks       []draftRewindPick `json:"picks"`
	Teams       []draftRewindTeam `json:"teams"`
}

type draftRewindPick struct {
	ID              uint   `json:"id"`
	PlayerID        uint   `json:"player_id"`
	Player          string `json:"player"`
	Position        string `json:"position"`
	TeamID          uint   `json:"team_id"`
	Round           uint   `json:"round"`
	Pick            uint   `json:"pick"`
	ObservedWeeks   int    `json:"observed_weeks"`
	ProjectionWeeks int    `json:"projection_weeks"`
	ScoreStatus     string `json:"score_status"`

	ActualPoints           *float64 `json:"actual_points"`
	ProjectedPoints        *float64 `json:"projected_points"`
	ProjectionActualPoints *float64 `json:"projection_actual_points"`
	RoundRank              *int     `json:"round_rank"`
	OverallRank            *int     `json:"overall_rank"`
}

type draftRewindTeam struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	TotalPicks  int    `json:"total_picks"`
	RankedPicks int    `json:"ranked_picks"`

	ActualPoints           *float64 `json:"actual_points"`
	ProjectedPicks         int      `json:"projected_picks"`
	ProjectedPoints        *float64 `json:"projected_points"`
	ProjectionActualPoints *float64 `json:"projection_actual_points"`
}

func draftLeague(c *gin.Context, db *gorm.DB) (uint, bool) {
	leagueID, ok := parseLeagueID(c)
	if !ok {
		return 0, false
	}
	if leagueID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid leagueId"})
		return 0, false
	}
	if err := db.Select("id").First(&models.League{}, leagueID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "league not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch league"})
		}
		return 0, false
	}
	return leagueID, true
}

func GetDraftSeasons(c *gin.Context) {
	db := database.DB.WithContext(c.Request.Context())
	leagueID, ok := draftLeague(c, db)
	if !ok {
		return
	}
	resp := GetLeagueYearsResponse{Years: []uint{}}
	if err := db.Model(&models.DraftSelection{}).Where("league_id = ?", leagueID).
		Distinct("year").Order("year DESC").Pluck("year", &resp.Years).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch draft seasons"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func GetDraftRewind(c *gin.Context) {
	year, err := strconv.ParseUint(c.Param("year"), 10, 32)
	if err != nil || year == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year"})
		return
	}
	db := database.DB.WithContext(c.Request.Context())
	leagueID, ok := draftLeague(c, db)
	if !ok {
		return
	}

	selections, err := models.GetLeagueDraftSelections(db, leagueID, uint(year))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch draft selections"})
		return
	}
	report := draftRewindReport{Year: uint(year), Picks: []draftRewindPick{}, Teams: []draftRewindTeam{}}
	if len(selections) == 0 {
		c.JSON(http.StatusOK, report)
		return
	}

	var schedule []models.Matchup
	if err := db.Select("id", "week", "year", "completed", "is_playoff", "game_type").
		Where("league_id = ? AND year = ?", leagueID, year).Find(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch draft scoring weeks"})
		return
	}
	matchupWeeks, throughWeek := draftScoringWeeks(schedule)
	report.ThroughWeek = throughWeek

	scores, err := loadDraftScores(db, selections, matchupWeeks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch draft scores"})
		return
	}
	report.addPicks(selections, scores)
	c.JSON(http.StatusOK, report)
}

func draftScoringWeeks(schedule []models.Matchup) (map[uint]uint, uint) {
	regularMatchups := make([]models.Matchup, 0, len(schedule))
	completedWeeks := make(map[uint]bool)
	for _, matchup := range schedule {
		if matchup.IsPlayoff || utils.GetPlayoffGameType(matchup, schedule) != utils.PlayoffGameTypeRegular {
			continue
		}
		regularMatchups = append(regularMatchups, matchup)
		// Any unfinished matchup holds back the entire week.
		if complete, exists := completedWeeks[matchup.Week]; !exists || complete {
			completedWeeks[matchup.Week] = matchup.Completed
		}
	}
	var throughWeek uint
	for completedWeeks[throughWeek+1] {
		throughWeek++
	}
	matchupWeeks := make(map[uint]uint)
	for _, matchup := range regularMatchups {
		if matchup.Week > 0 && matchup.Week <= throughWeek {
			matchupWeeks[matchup.ID] = matchup.Week
		}
	}
	return matchupWeeks, throughWeek
}

type draftWeekScore struct {
	actual, projected                        int64
	conflictingActual, conflictingProjection bool
}

func loadDraftScores(db *gorm.DB, selections []models.DraftSelection, matchups map[uint]uint) (map[uint]map[uint]draftWeekScore, error) {
	scores := make(map[uint]map[uint]draftWeekScore)
	if len(matchups) == 0 {
		return scores, nil
	}

	matchupIDs := make([]uint, 0, len(matchups))
	for id := range matchups {
		matchupIDs = append(matchupIDs, id)
	}
	playerIDs := make([]uint, 0, len(selections))
	for _, selection := range selections {
		if selection.PlayerID != 0 {
			playerIDs = append(playerIDs, selection.PlayerID)
		}
	}
	var rows []models.BoxScore
	if err := db.Select("matchup_id", "player_id", "actual_points", "projected_points").
		Where("matchup_id IN ? AND player_id IN ?", matchupIDs, playerIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		playerWeeks := scores[row.PlayerID]
		if playerWeeks == nil {
			playerWeeks = make(map[uint]draftWeekScore)
			scores[row.PlayerID] = playerWeeks
		}
		week := matchups[row.MatchupID]
		actual := int64(math.Round(row.ActualPoints * 100))
		projected := int64(math.Round(row.ProjectedPoints * 100))
		score, exists := playerWeeks[week]
		if exists {
			score.conflictingActual = score.conflictingActual || score.actual != actual
			score.conflictingProjection = score.conflictingProjection || score.projected != projected
		} else {
			score = draftWeekScore{actual: actual, projected: projected}
		}
		playerWeeks[week] = score
	}
	return scores, nil
}

func (pick *draftRewindPick) addScores(scores map[uint]draftWeekScore) {
	pick.ObservedWeeks = len(scores)
	pick.ScoreStatus = "no_scores"
	if len(scores) == 0 {
		return
	}
	var actual, projected, pairedActual int64
	var projectionWeeks int
	for _, score := range scores {
		if score.conflictingActual {
			pick.ScoreStatus = "conflicting_scores"
			return
		}
		actual += score.actual
		// The stored zero default cannot distinguish a missing projection.
		if score.projected != 0 && !score.conflictingProjection {
			projected += score.projected
			pairedActual += score.actual
			projectionWeeks++
		}
	}
	actualPoints := float64(actual) / 100
	pick.ActualPoints = &actualPoints
	pick.ScoreStatus = "ranked"
	if projectionWeeks > 0 {
		projectedPoints := float64(projected) / 100
		projectionActualPoints := float64(pairedActual) / 100
		pick.ProjectedPoints = &projectedPoints
		pick.ProjectionActualPoints = &projectionActualPoints
		pick.ProjectionWeeks = projectionWeeks
	}
}

func (report *draftRewindReport) addPicks(selections []models.DraftSelection, scores map[uint]map[uint]draftWeekScore) {
	teamIndices := make(map[uint]int)
	ranked := make([]int, 0, len(selections))
	for _, selection := range selections {
		pick := draftRewindPick{
			ID: selection.ID, PlayerID: selection.PlayerID, Player: selection.PlayerName,
			Position: selection.PlayerPosition, TeamID: selection.TeamID, Round: selection.Round, Pick: selection.Pick,
		}
		pick.addScores(scores[selection.PlayerID])
		index, exists := teamIndices[pick.TeamID]
		if !exists {
			name := "Unknown team"
			if selection.Team != nil {
				name = cmp.Or(selection.Team.Name, selection.Team.Owner, name)
			}
			index = len(report.Teams)
			teamIndices[pick.TeamID] = index
			report.Teams = append(report.Teams, draftRewindTeam{ID: pick.TeamID, Name: name})
		}
		report.Teams[index].addPick(pick)
		if pick.ActualPoints != nil {
			ranked = append(ranked, len(report.Picks))
		}
		report.Picks = append(report.Picks, pick)
	}

	slices.SortStableFunc(ranked, func(a, b int) int {
		return cmp.Compare(*report.Picks[b].ActualPoints, *report.Picks[a].ActualPoints)
	})
	var overall draftRank
	rounds := make(map[uint]draftRank)
	for _, index := range ranked {
		pick := &report.Picks[index]
		round := rounds[pick.Round]
		pick.OverallRank = overall.next(*pick.ActualPoints)
		pick.RoundRank = round.next(*pick.ActualPoints)
		rounds[pick.Round] = round
	}
	report.TotalPicks = len(report.Picks)
	report.RankedPicks = len(ranked)
}

type draftRank struct {
	count  int
	rank   *int
	points float64
}

func (r *draftRank) next(points float64) *int {
	r.count++
	if r.rank == nil || r.points != points {
		rank := r.count
		r.rank, r.points = &rank, points
	}
	return r.rank
}

func (team *draftRewindTeam) addPick(pick draftRewindPick) {
	team.TotalPicks++
	if pick.ActualPoints != nil {
		team.RankedPicks++
		addDraftPoints(&team.ActualPoints, *pick.ActualPoints)
	}
	if pick.ProjectedPoints != nil {
		team.ProjectedPicks++
		addDraftPoints(&team.ProjectedPoints, *pick.ProjectedPoints)
		addDraftPoints(&team.ProjectionActualPoints, *pick.ProjectionActualPoints)
	}
}

func addDraftPoints(total **float64, points float64) {
	if *total == nil {
		*total = &points
		return
	}
	**total = math.Round((**total+points)*100) / 100
}

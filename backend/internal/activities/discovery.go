package activities

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/helpers"
	"backend/internal/models"
	"backend/internal/sleeper"
)

// DiscoveryLogTag is stamped on every discovery-related log line (activity
// and workflow) as a "tag" field, so a single grep on the shared worker
// journal — which also carries draft-sync, transaction-sync, and ESPN
// worker log lines — pulls out exactly the discovery pipeline's output:
//
//	journalctl -u ff-sims-worker | grep discovery_trace
const DiscoveryLogTag = "discovery_trace"

// firstScannedSeason is the earliest NFL season scraped per user discovery run.
// Older seasons are excluded — their data is complete and not worth re-scanning.
const firstScannedSeason = 2025

// Seasons returns the NFL seasons to scan during user discovery: firstScannedSeason
// through the current calendar year. Computed at call time (rather than a fixed
// list) so next season's leagues are picked up automatically without a yearly
// code change.
func Seasons() []string {
	currentYear := time.Now().Year()
	seasons := make([]string, 0, currentYear-firstScannedSeason+1)
	for y := firstScannedSeason; y <= currentYear; y++ {
		seasons = append(seasons, strconv.Itoa(y))
	}
	return seasons
}

// DiscoveryActivities holds dependencies for user/league graph expansion activities.
type DiscoveryActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// GetDiscoveryConfig returns the discovery dispatcher tuning knobs from env,
// clamped to at least 1 so a bad value can't stall dispatch or break the
// claim query's LIMIT.
func (a *DiscoveryActivities) GetDiscoveryConfig(ctx context.Context) (DiscoveryConfig, error) {
	return DiscoveryConfig{
		ParallelBatches:    max(helpers.GetEnv("DISCOVERY_PARALLEL_BATCHES", 1), 1),
		BatchSize:          max(helpers.GetEnv("DISCOVERY_BATCH_SIZE", 20), 1),
		Concurrency:        max(helpers.GetEnv("DISCOVERY_USER_CONCURRENCY", 4), 1),
		UserTimeoutSeconds: max(helpers.GetEnv("DISCOVERY_USER_TIMEOUT_SECONDS", 90), 1),
		LeagueConcurrency:  max(helpers.GetEnv("DISCOVERY_LEAGUE_CONCURRENCY", 10), 1),
	}, nil
}

// claimStaleUsersSQL atomically claims up to batchSize stale users for
// discovery (same pattern as the league sync paths). FOR UPDATE SKIP LOCKED
// lets concurrent claimers partition the queue without double-claiming, and
// the 20-minute expiry re-queues users claimed by a worker that died
// mid-batch. Because ticks claim rather than re-select, a stuck cohort can
// never head-of-line-block the queue the way the old workflow-ID-collision
// dedupe did.
const claimStaleUsersSQL = `
UPDATE sleeper_users SET claimed_at = now()
WHERE sleeper_user_id IN (
    SELECT sleeper_user_id FROM sleeper_users
    WHERE skipped_at IS NULL
      AND (claimed_at IS NULL OR claimed_at < now() - interval '20 minutes')
    ORDER BY last_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_user_id`

// ClaimStaleUsers claims up to BatchSize users for discovery, never-fetched
// first then oldest. Postgres-only (SKIP LOCKED).
func (a *DiscoveryActivities) ClaimStaleUsers(ctx context.Context, params ClaimStaleUsersParams) ([]string, error) {
	var ids []string
	if err := a.DB.WithContext(ctx).Raw(claimStaleUsersSQL, params.BatchSize).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// DiscoverUsersBatch runs discovery for a claimed batch of users with bounded
// concurrency, stamping each user done as it completes. Per-user failures are
// counted, not propagated: a failed user keeps its claim and re-enters the
// queue when the claim expires. The activity heartbeats as users complete so
// a dead worker is detected via HeartbeatTimeout.
//
// Each user gets its own UserTimeoutSeconds sub-context. wg.Wait below blocks
// until every user in the batch finishes, so without a per-user bound, one
// user stuck behind slow Sleeper responses (many leagues, or upstream
// degradation) stalls the other 49 and can drag the whole activity past its
// StartToCloseTimeout. Bounding each user lets the batch make forward
// progress on the rest; the stuck user just keeps its claim and gets retried
// once it expires.
func (a *DiscoveryActivities) DiscoverUsersBatch(ctx context.Context, params DiscoverUsersBatchParams) (SyncBatchResult, error) {
	logger := activity.GetLogger(ctx)
	res := SyncBatchResult{}
	batchStart := time.Now()

	// Re-scope to users still claimed: on an activity retry, users stamped by
	// the previous attempt have claimed_at cleared and must not re-run.
	var stillClaimed []string
	if err := a.DB.WithContext(ctx).Model(&models.SleeperUser{}).
		Where("sleeper_user_id IN ? AND claimed_at IS NOT NULL", params.UserIDs).
		Pluck("sleeper_user_id", &stillClaimed).Error; err != nil {
		return res, err
	}
	if len(stillClaimed) == 0 {
		return res, nil
	}

	concurrency := max(1, params.Concurrency)
	userTimeout := time.Duration(max(1, params.UserTimeoutSeconds)) * time.Second
	leagueConcurrency := max(1, params.LeagueConcurrency)
	logger.Info("discovery batch starting", "tag", DiscoveryLogTag,
		"userCount", len(stillClaimed), "concurrency", concurrency,
		"leagueConcurrency", leagueConcurrency, "userTimeoutSeconds", params.UserTimeoutSeconds)

	type userResult struct {
		userID      string
		err         error
		duration    time.Duration
		leagueCount int
	}
	sem := make(chan struct{}, concurrency)
	results := make(chan userResult, len(stillClaimed))
	var wg sync.WaitGroup
	for _, id := range stillClaimed {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			userCtx, cancel := context.WithTimeout(ctx, userTimeout)
			defer cancel()
			start := time.Now()
			leagueCount, err := a.discoverOneUser(userCtx, logger, id, leagueConcurrency)
			results <- userResult{userID: id, err: err, duration: time.Since(start), leagueCount: leagueCount}
		})
	}
	go func() { wg.Wait(); close(results) }()

	// Heartbeat on every result rather than throttling by count: with a
	// small Concurrency (e.g. 4) that doesn't evenly divide a fixed
	// throttle interval, results can arrive in synchronized clusters that
	// never land on the throttle boundary — see the incident this fixes,
	// where a batch of uniformly-slow users produced zero heartbeats for
	// its entire duration and tripped HeartbeatTimeout despite the activity
	// actively working the whole time. RecordHeartbeat is cheap; the SDK
	// itself throttles the actual server RPC, so callers don't need to.
	done := 0
	for r := range results {
		done++
		if r.err != nil {
			res.Failed++
			logger.Warn("user discovery failed", "tag", DiscoveryLogTag,
				"userID", r.userID, "error", r.err, "duration", r.duration, "leagueCount", r.leagueCount)
		} else {
			res.Processed++
			logger.Info("user discovery completed", "tag", DiscoveryLogTag,
				"userID", r.userID, "duration", r.duration, "leagueCount", r.leagueCount)
		}
		activity.RecordHeartbeat(ctx, done)
	}

	logger.Info("discovery batch finished", "tag", DiscoveryLogTag,
		"userCount", len(stillClaimed), "processed", res.Processed, "failed", res.Failed,
		"duration", time.Since(batchStart))
	return res, nil
}

// discoverOneUser fetches a user's leagues across configured seasons, then for
// each league upserts its members (with NULL last_fetched_at so they enter the
// discovery queue) and its details, and finally stamps the user done (clearing
// the claim). Per-league failures are logged and skipped, matching the old
// UserDiscoveryWorkflow's warn-and-continue behavior. A 404 on the user marks
// them permanently skipped.
//
// Leagues already complete-and-fetched are skipped entirely (their metadata
// and membership are immutable), and the remaining leagues are fetched with
// up to leagueConcurrency in flight at once rather than one at a time. Some
// Sleeper users belong to hundreds of leagues — at strictly one league per
// round trip, such a user could never finish within any reasonable per-user
// timeout, would never get last_fetched_at stamped, and (since ClaimStaleUsers
// orders never-fetched users first) would permanently squat at the head of
// the discovery queue, retried every claim cycle without ever making
// progress. Fanning out league fetches turns "N leagues x 2 sequential
// round trips" into roughly "N/leagueConcurrency rounds".
//
// Returns the number of leagues discovered for userID (regardless of how
// many were skipped, fetched, or failed) so callers can log it alongside
// the outcome — a cheap way to spot mega-league users straight from
// activity logs without a DB query.
func (a *DiscoveryActivities) discoverOneUser(ctx context.Context, logger log.Logger, userID string, leagueConcurrency int) (int, error) {
	leagueIDs, err := a.FetchUserLeagues(ctx, FetchUserLeaguesParams{UserID: userID})
	if err != nil {
		if isNotFoundAppError(err) {
			return 0, a.DB.WithContext(ctx).
				Model(&models.SleeperUser{}).
				Where("sleeper_user_id = ?", userID).
				Updates(map[string]interface{}{
					"skipped_at": time.Now().UTC(),
					"claimed_at": nil,
				}).Error
		}
		return 0, err
	}

	skipped := 0
	sem := make(chan struct{}, max(1, leagueConcurrency))
	var wg sync.WaitGroup
	for _, lid := range leagueIDs {
		// Once ctx is done (user or activity deadline hit), every remaining
		// Sleeper call would fail instantly on ctx.Err() without touching the
		// network — dispatching the rest of leagueIDs anyway just burns CPU
		// and floods the log with one WARN line per remaining league. Stop
		// dispatching new work and let what's already in flight wind down.
		if ctx.Err() != nil {
			break
		}
		if a.leagueFullySynced(ctx, lid) {
			skipped++
			continue
		}
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			leagueStart := time.Now()
			if err := a.FetchLeagueMembers(ctx, FetchLeagueMembersParams{LeagueID: lid}); err != nil {
				logger.Warn("FetchLeagueMembers failed, continuing", "tag", DiscoveryLogTag, "leagueID", lid, "error", err)
			}
			if err := a.FetchLeagueDetails(ctx, FetchLeagueDetailsParams{LeagueID: lid}); err != nil {
				logger.Warn("FetchLeagueDetails failed, continuing", "tag", DiscoveryLogTag, "leagueID", lid, "error", err)
			}
			if d := time.Since(leagueStart); d > 5*time.Second {
				logger.Warn("slow league fetch", "tag", DiscoveryLogTag, "leagueID", lid, "userID", userID, "duration", d)
			}
		})
	}
	wg.Wait()
	logger.Info("user leagues resolved", "tag", DiscoveryLogTag,
		"userID", userID, "leagueCount", len(leagueIDs), "skippedAlreadySynced", skipped)
	if ctx.Err() != nil {
		return len(leagueIDs), ctx.Err()
	}

	return len(leagueIDs), a.DB.WithContext(ctx).
		Model(&models.SleeperUser{}).
		Where("sleeper_user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_fetched_at": time.Now().UTC(),
			"claimed_at":      nil,
		}).Error
}

// isNotFoundAppError reports whether err is the NOT_FOUND application error
// produced by the fetch helpers when a Sleeper entity no longer exists.
func isNotFoundAppError(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "NOT_FOUND"
}

// FetchUserLeagues fetches all leagues for userID across all configured seasons,
// upserts them into sleeper_leagues and sleeper_league_users, and returns the discovered league IDs.
// Returns a non-retryable NOT_FOUND error if the user no longer exists in Sleeper.
func (a *DiscoveryActivities) FetchUserLeagues(ctx context.Context, params FetchUserLeaguesParams) ([]string, error) {
	var leagueIDs []string
	for _, season := range Seasons() {
		leagues, err := a.Sleeper.GetUserLeagues(ctx, params.UserID, "nfl", season)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				return nil, temporal.NewNonRetryableApplicationError(
					"user not found: "+params.UserID, "NOT_FOUND", err,
				)
			}
			return nil, err
		}
		for _, l := range leagues {
			row := models.SleeperLeague{
				SleeperLeagueID: l.LeagueID,
				Name:            l.Name,
				Season:          l.Season,
				Sport:           l.Sport,
				Status:          l.Status,
				TotalRosters:    l.TotalRosters,
			}
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&row).Error; err != nil {
				return nil, err
			}
			junc := models.SleeperLeagueUser{
				SleeperLeagueID: l.LeagueID,
				SleeperUserID:   params.UserID,
			}
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&junc).Error; err != nil {
				return nil, err
			}
			leagueIDs = append(leagueIDs, l.LeagueID)
		}
	}
	return leagueIDs, nil
}

// FetchLeagueMembers fetches all members of leagueID and upserts them as new sleeper_users
// with last_fetched_at=NULL so they are picked up by future dispatcher runs.
func (a *DiscoveryActivities) FetchLeagueMembers(ctx context.Context, params FetchLeagueMembersParams) error {
	users, err := a.Sleeper.GetLeagueUsers(ctx, params.LeagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return temporal.NewNonRetryableApplicationError(
				"league not found: "+params.LeagueID, "NOT_FOUND", err,
			)
		}
		return err
	}
	for _, u := range users {
		row := models.SleeperUser{
			SleeperUserID: u.UserID,
			Username:      u.Username,
			DisplayName:   u.DisplayName,
			Avatar:        u.Avatar,
		}
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// sleeperLeagueType converts the integer type from Sleeper's league settings to a string.
// Sleeper encodes: 0=redraft, 1=keeper, 2=dynasty.
func sleeperLeagueType(t int) string {
	switch t {
	case 1:
		return "keeper"
	case 2:
		return "dynasty"
	default:
		return "redraft"
	}
}

// leagueFullySynced reports whether leagueID is marked complete with details
// already fetched — a completed league's metadata and membership are both
// immutable, so neither needs to be re-fetched on future discovery passes.
func (a *DiscoveryActivities) leagueFullySynced(ctx context.Context, leagueID string) bool {
	var existing models.SleeperLeague
	if err := a.DB.WithContext(ctx).
		Where("sleeper_league_id = ?", leagueID).
		First(&existing).Error; err != nil {
		return false
	}
	return existing.Status == "complete" && existing.LastFetchedAt != nil
}

// FetchLeagueDetails fetches league metadata from Sleeper and stamps last_fetched_at.
// Called during user discovery so league metadata is populated before draft/transaction sync.
// Skips the API call for leagues already marked complete — their metadata is immutable.
func (a *DiscoveryActivities) FetchLeagueDetails(ctx context.Context, params FetchLeagueDetailsParams) error {
	if a.leagueFullySynced(ctx, params.LeagueID) {
		return nil
	}

	league, err := a.Sleeper.GetLeague(ctx, params.LeagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return temporal.NewNonRetryableApplicationError(
				"league not found: "+params.LeagueID, "NOT_FOUND", err,
			)
		}
		return err
	}

	scoringJSON, _ := json.Marshal(league.ScoringSettings)
	rosterJSON, _ := json.Marshal(league.RosterPositions)

	ppr := league.ScoringSettings["rec"]
	tePremium := league.ScoringSettings["bonus_rec_te"]
	isSuperflex := false
	for _, pos := range league.RosterPositions {
		if pos == "SUPER_FLEX" {
			isSuperflex = true
			break
		}
	}

	leagueType := sleeperLeagueType(league.Settings.Type)

	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Updates(map[string]interface{}{
			"name":             league.Name,
			"status":           league.Status,
			"total_rosters":    league.TotalRosters,
			"ppr":              ppr,
			"te_premium":       tePremium,
			"is_superflex":     isSuperflex,
			"league_type":      leagueType,
			"scoring_settings": scoringJSON,
			"roster_positions": rosterJSON,
			"last_fetched_at":  now,
		}).Error
}

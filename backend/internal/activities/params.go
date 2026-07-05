package activities

import "backend/internal/models"

type GetStaleUsersParams struct {
	BatchSize int
}

type FetchUserLeaguesParams struct {
	UserID string
}

type FetchLeagueMembersParams struct {
	LeagueID string
}

type MarkUserFetchedParams struct {
	UserID string
}

type MarkUserSkippedParams struct {
	UserID string
}

type GetStaleLeaguesParams struct {
	BatchSize int
}

type FetchLeagueDetailsParams struct {
	LeagueID string
}

type FetchLeagueDraftsParams struct {
	LeagueID string
}

type FetchDraftPicksParams struct {
	DraftID string
}

type FetchLeagueTransactionsParams struct {
	LeagueID       string
	LastLegFetched *int
}

type ClaimLeaguesForTransactionsParams struct {
	BatchSize int
}

// LeagueTransactionState carries the league ID, season, and leg cursor for one
// claimed league, as returned by ClaimLeaguesForTransactions.
type LeagueTransactionState struct {
	LeagueID       string
	Season         string
	LastLegFetched *int
}

type MarkLeagueFetchedParams struct {
	LeagueID string
}

type MarkLeagueTransactionsFetchedParams struct {
	LeagueID string
	MaxLeg   int
}

type MarkLeagueSkippedParams struct {
	LeagueID string
}

type FetchWeekStatsParams struct {
	Season string
	Week   int
}

type GetFinalizedWeeksParams struct {
	Season string
}

type ComputeSegmentSeasonADPParams struct {
	Segment models.ADPSegment
	Season  string
}

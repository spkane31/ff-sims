package activities

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

// LeagueTransactionState carries the league ID and leg cursor returned by GetStaleLeaguesForTransactions.
type LeagueTransactionState struct {
	LeagueID       string
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

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
	LeagueID string
}

type MarkLeagueFetchedParams struct {
	LeagueID string
}

type MarkLeagueSkippedParams struct {
	LeagueID string
}

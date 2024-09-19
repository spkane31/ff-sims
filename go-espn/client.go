package espn

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	_ "github.com/joho/godotenv/autoload"
)

var fantasySportsAbbreviation = map[string]string{
	"nfl":  "ffl",
	"nba":  "fba",
	"nhl":  "fhl",
	"mlb":  "flb",
	"wnba": "wfba",
}

type Client struct {
	baseURL  string
	espns2   string
	swid     string
	year     int
	sport    string
	leagueID int
}

type ClientOpts struct {
	BaseURL string
	ESPNS2  string
	SWID    string
	Year    int
	Sport   string
}

func DefaultClientOpts() *ClientOpts {
	return &ClientOpts{
		BaseURL: "https://lm-api-reads.fantasy.espn.com/apis/v3/games/",
		Year:    2024,
		Sport:   "nfl",
		SWID:    os.Getenv("SWID"),
		ESPNS2:  os.Getenv("ESPN_S2"),
	}
}

func NewClient(leagueID int, opts *ClientOpts) *Client {
	if opts == nil {
		opts = DefaultClientOpts()
	}
	if opts.BaseURL == "" {
		opts.BaseURL = "https://lm-api-reads.fantasy.espn.com/apis/v3/games/"
	}
	if opts.Year == 0 {
		opts.Year = 2024
	}
	if opts.Sport == "" {
		opts.Sport = "nfl"
	}
	if opts.SWID == "" {
		opts.SWID = os.Getenv("SWID")
	}
	if opts.ESPNS2 == "" {
		opts.ESPNS2 = os.Getenv("ESPN_S2")
	}

	return &Client{
		baseURL:  opts.BaseURL,
		espns2:   opts.ESPNS2,
		swid:     opts.SWID,
		year:     opts.Year,
		sport:    opts.Sport,
		leagueID: leagueID,
	}
}

func (c *Client) createRequest() string {
	leagueEndpoint := c.baseURL + fantasySportsAbbreviation[c.sport]

	if c.year < 2018 {
		leagueEndpoint += fmt.Sprintf("/leagueHistory/%d?seasonId=%d", c.leagueID, c.year)
	} else {
		leagueEndpoint += fmt.Sprintf("/seasons/%d/segments/0/leagues/%d", c.year, c.leagueID)
	}
	return leagueEndpoint
}

func (c *Client) GetLeague() error {
	leagueEndpoint := c.createRequest()

	baseURL, err := url.Parse(leagueEndpoint)
	if err != nil {
		return err
	}

	params := url.Values{}
	if c.year < 2018 {
		params.Add("seasonId", fmt.Sprintf("%d", c.year))
	}
	params.Add("view", "mTeam")
	params.Add("view", "mRoster")
	params.Add("view", "mMatchup")
	params.Add("view", "mSettings")
	params.Add("view", "mStandings")
	// 'view': ['mTeam', 'mRoster', 'mMatchup', 'mSettings', 'mStandings']
	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", baseURL.String(), nil)
	if err != nil {
		return err
	}

	cookies := []*http.Cookie{
		{
			Name:  "espn_s2",
			Value: c.espns2,
		},
		{
			Name:  "SWID",
			Value: c.swid,
		},
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// read http response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println(string(body))

	return nil
}

type TeamResponse struct {
	GameID   int      `json:"gameId"`
	LeagueID int      `json:"id"`
	Members  []Member `json:"members"`
	Status   Status   `json:"status"`
	Team     []Team   `json:"teams"`
}

type Member struct {
	DisplayName string `json:"displayName"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
}

type Status struct {
	ActivatedDate        uint  `json:"activatedDate"`
	CurrentMatchupPeriod int   `json:"currentMatchupPeriod"`
	PreviousSeasons      []int `json:"previousSeasons"`
}

type Team struct {
	Abbreviation          string     `json:"abbrev"`
	ID                    int        `json:"id"`
	Name                  string     `json:"location"`
	WaiverRank            int        `json:"waiverRank"`
	CurrentProjectedRank  int        `json:"currentProjectedRank"`
	DraftDayProjectedRank int        `json:"draftDayProjectedRank"`
	Logo                  string     `json:"logo"`
	TradeBlock            TradeBlock `json:"tradeBlock"`
}

type TradeBlock struct {
	Players map[string]string `json:"players"`
}

func (c *Client) GetTeams() (TeamResponse, error) {
	leagueEndpoint := c.createRequest()

	baseURL, err := url.Parse(leagueEndpoint)
	if err != nil {
		return TeamResponse{}, err
	}

	params := url.Values{}
	if c.year < 2018 {
		params.Add("seasonId", fmt.Sprintf("%d", c.year))
	}
	params.Add("view", "mTeam")
	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", baseURL.String(), nil)
	if err != nil {
		return TeamResponse{}, err
	}

	cookies := []*http.Cookie{
		{
			Name:  "espn_s2",
			Value: c.espns2,
		},
		{
			Name:  "SWID",
			Value: c.swid,
		},
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TeamResponse{}, err
	}

	if resp.StatusCode != 200 {
		return TeamResponse{}, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// read http response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TeamResponse{}, err
	}

	var ret TeamResponse
	if err := json.Unmarshal(body, &ret); err != nil {
		return TeamResponse{}, err
	}

	return ret, nil
}

type DraftDetail struct {
	DraftComplete bool `json:"drafted"`
	InProgress    bool `json:"inProgress"`
}

type RosterResponse struct {
	DraftDetail DraftDetail  `json:"draftDetail"`
	GameID      int          `json:"gameId"`
	LeagueID    int          `json:"id"`
	SeasonID    int          `json:"seasonId"`
	Status      Status       `json:"status"`
	Teams       []TeamRoster `json:"teams"`
}

type TeamRoster struct {
	ID     int    `json:"id"`
	Roster Roster `json:"roster"`
}

type Roster struct {
	Entries []RosterEntry `json:"entries"`
}

type RosterEntry struct {
	AcquisitionDate uint   `json:"acquisitionDate"`
	PlayerID        int    `json:"playerId"`
	Name            string `json:"player"`
	InjuryStatus    string `json:"injuryStatus"`
	ProTeamID       int    `json:"proTeamId"`
	Stats           []Stat `json:"stats"`
}

type Stat struct {
	AppliedAverage float64 `json:"appliedAverage"`
	AppliedTotal   float64 `json:"appliedTotal"`
}

func (c *Client) GetRosters() (RosterResponse, error) {
	leagueEndpoint := c.createRequest()

	baseURL, err := url.Parse(leagueEndpoint)
	if err != nil {
		return RosterResponse{}, err
	}

	params := url.Values{}
	if c.year < 2018 {
		params.Add("seasonId", fmt.Sprintf("%d", c.year))
	}
	params.Add("view", "mRoster")
	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", baseURL.String(), nil)
	if err != nil {
		return RosterResponse{}, err
	}

	cookies := []*http.Cookie{
		{
			Name:  "espn_s2",
			Value: c.espns2,
		},
		{
			Name:  "SWID",
			Value: c.swid,
		},
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return RosterResponse{}, err
	}

	if resp.StatusCode != 200 {
		return RosterResponse{}, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// read http response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RosterResponse{}, err
	}

	var ret RosterResponse
	if err := json.Unmarshal(body, &ret); err != nil {
		return RosterResponse{}, err
	}

	return ret, nil
}

type MatchupResponse struct {
	DraftDetail DraftDetail `json:"draftDetail"`
	GameID      int         `json:"gameId"`
}

func (c *Client) GetMatchups() (MatchupResponse, error) {
	leagueEndpoint := c.createRequest()

	baseURL, err := url.Parse(leagueEndpoint)
	if err != nil {
		return MatchupResponse{}, err
	}

	params := url.Values{}
	if c.year < 2018 {
		params.Add("seasonId", fmt.Sprintf("%d", c.year))
	}
	params.Add("view", "mMatchup")
	// params.Add("view", "mSettings")
	// params.Add("view", "mStandings")
	// 'view': ['mTeam', 'mRoster', 'mMatchup', 'mSettings', 'mStandings']
	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", baseURL.String(), nil)
	if err != nil {
		return MatchupResponse{}, err
	}

	cookies := []*http.Cookie{
		{
			Name:  "espn_s2",
			Value: c.espns2,
		},
		{
			Name:  "SWID",
			Value: c.swid,
		},
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return MatchupResponse{}, err
	}

	if resp.StatusCode != 200 {
		return MatchupResponse{}, fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// read http response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MatchupResponse{}, err
	}

	var ret MatchupResponse
	if err := json.Unmarshal(body, &ret); err != nil {
		return MatchupResponse{}, err
	}

	fmt.Println(string(body))

	return ret, nil
}

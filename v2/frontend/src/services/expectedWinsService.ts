import { apiClient } from "./apiClient";

// Expected wins data interfaces
export interface WeeklyExpectedWins {
  id: number;
  team_id: number;
  week: number;
  year: number;
  league_id: number;
  expected_wins: number;
  weekly_expected_wins: number;
  expected_losses: number;
  weekly_expected_losses: number;
  actual_wins: number;
  actual_losses: number;
  weekly_actual_win: boolean;
  win_luck: number;
  strength_of_schedule: number;
  weekly_win_probability: number;
  team_score: number;
  opponent_score: number;
  opponent_team_id: number;
  point_differential: number;
  team?: {
    id: number;
    name: string;
    owner_name: string;
  };
}

export interface SeasonExpectedWins {
  id: number;
  team_id: number;
  year: number;
  league_id: number;
  expected_wins: number;
  expected_losses: number;
  actual_wins: number;
  actual_losses: number;
  win_luck: number;
  strength_of_schedule: number;
  team?: {
    id: number;
    name: string;
    owner_name: string;
  };
}

export interface AllTimeExpectedWins {
  team_id: number;
  team_name: string;
  owner: string;
  total_expected_wins: number;
  total_expected_losses: number;
  total_actual_wins: number;
  total_actual_losses: number;
  total_win_luck: number;
  seasons_played: number;
}

export interface WeeklyExpectedWinsResponse {
  data: WeeklyExpectedWins[];
}

export interface SeasonExpectedWinsResponse {
  data: SeasonExpectedWins[];
}

export interface TeamProgressionResponse {
  data: WeeklyExpectedWins[];
}

export interface AllTimeExpectedWinsResponse {
  data: AllTimeExpectedWins[];
}

export interface CurrentSeasonStanding {
  team_id: number;
  espn_id: string;
  owner: string;
  team_name: string;
  record: {
    wins: number;
    losses: number;
    ties: number;
  };
  points: {
    scored: number;
    against: number;
  };
  expected_wins?: number;
  expected_losses?: number;
  win_luck?: number;
}

export interface CurrentSeasonStandingsResponse {
  year: number;
  standings: CurrentSeasonStanding[];
}

/**
 * Expected Wins API service
 */
export const expectedWinsService = {
  getAllTimeExpectedWins: async (leagueId: number): Promise<AllTimeExpectedWinsResponse> => {
    return apiClient.get<AllTimeExpectedWinsResponse>(`/leagues/${leagueId}/teams/all-time-expected-wins`);
  },

  getWeeklyExpectedWins: async (leagueId: number, year: number, week?: number): Promise<WeeklyExpectedWinsResponse> => {
    const params = week ? `?week=${week}` : '';
    return apiClient.get<WeeklyExpectedWinsResponse>(`/leagues/${leagueId}/expected-wins/weekly/${year}${params}`);
  },

  getSeasonExpectedWins: async (leagueId: number, year: number): Promise<SeasonExpectedWinsResponse> => {
    return apiClient.get<SeasonExpectedWinsResponse>(`/leagues/${leagueId}/expected-wins/season/${year}`);
  },

  getTeamProgression: async (leagueId: number, teamId: number, year: number): Promise<TeamProgressionResponse> => {
    return apiClient.get<TeamProgressionResponse>(`/leagues/${leagueId}/teams/${teamId}/expected-wins/${year}`);
  },

  getCurrentSeasonStandings: async (leagueId: number, year: number): Promise<CurrentSeasonStandingsResponse> => {
    return apiClient.get<CurrentSeasonStandingsResponse>(`/leagues/${leagueId}/teams/standings/${year}`);
  },
};
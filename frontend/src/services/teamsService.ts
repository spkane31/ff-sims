import { apiClient } from "./apiClient";

// Define TypeScript interfaces for your team data
export interface TeamRecord {
  wins: number;
  losses: number;
  ties: number;
}

export interface TeamPoints {
  scored: number;
  against: number;
}

export interface TeamExpectedWins {
  expectedWins: number;
  expectedLosses: number;
  winLuck: number;
  seasonsPlayed: number;
}

export interface Team {
  id: string;
  espnId: string;
  name: string;
  owner: string;
  record: TeamRecord;
  playoffRecord: TeamRecord;
  points: TeamPoints;
  expectedWins?: TeamExpectedWins;
  rank: number;
  playoffChance: number;
}

export interface TeamResponse {
  teams: Team[];
}

// Extended interfaces for detailed team view
export interface PlayerStats {
  passingYards: number;
  passingTDs: number;
  interceptions: number;
  rushingYards: number;
  rushingTDs: number;
  receptions: number;
  receivingYards: number;
  receivingTDs: number;
  fumbles: number;
  fieldGoals: number;
  extraPoints: number;
}

export interface Player {
  id: string;
  name: string;
  position: string;
  team: string;
  status: string;
  fantasyPoints: number;
  stats: PlayerStats;
}

export interface DraftPick {
  round: number;
  pick: number;
  player: string;
  position: string;
  team: string;
  year: number;
}

export interface Transaction {
  id: string;
  type: string;
  date: string;
  year: number;
  week: number;
  description: string;
  playersGained: {
    id: string;
    name: string;
  }[];
  playersLost: {
    id: string;
    name: string;
  }[];
}

export interface ScheduleGame {
  week: number;
  year: number;
  opponent: string;
  opponentESPNID: string; // Add opponent ESPN ID for linking
  isHome: boolean;
  teamScore: number;
  opponentScore: number;
  result: string; // "W", "L", "T", or "Upcoming"
  completed: boolean;
  isPlayoff: boolean;
  matchupId?: string; // Add matchup ID for linking to schedule detail page
}

export interface TeamDetail {
  id: string;
  espnId: string;
  name: string;
  owner: string;
  record: TeamRecord;
  points: TeamPoints;
  schedule: ScheduleGame[];
  currentPlayers: Player[];
  draftPicks: DraftPick[];
  transactions: Transaction[];
}

/**
 * Teams API service
 */
export const teamsService = {
  getAllTeams: async (leagueId: number): Promise<TeamResponse> => {
    return apiClient.get<TeamResponse>(`/leagues/${leagueId}/teams`);
  },

  getTeamById: async (leagueId: number, teamId: number | string): Promise<Team> => {
    return apiClient.get<Team>(`/leagues/${leagueId}/teams/${teamId}`);
  },

  getTeamDetail: async (leagueId: number, teamId: string | number): Promise<TeamDetail> => {
    return apiClient.get<TeamDetail>(`/leagues/${leagueId}/teams/${teamId}`);
  },

  getTeamStandings: async (leagueId: number, year: number): Promise<Team[]> => {
    return apiClient.get<Team[]>(`/leagues/${leagueId}/teams/standings/${year}`);
  },
};

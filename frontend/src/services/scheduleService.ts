import { apiClient } from "./apiClient";
import { Matchup, GetMatchupsResponse } from "../types/models";

// Legacy interfaces for backward compatibility
export interface Game {
  id: number;
  year: number;
  week: number;
  homeTeamId: number;
  awayTeamId: number;
  homeTeamOwner: string;
  awayTeamOwner: string;
  homeTeamScore: number;
  awayTeamScore: number;
  homeTeamProjectedScore: number;
  awayTeamProjectedScore: number;
  completed: boolean;
}

export interface GetScheduleResponse {
  data: Schedule;
}

export interface Schedule {
  matchups: Matchup[];
}

export interface Player {
  id: string;
  playerName: string;
  playerPosition: string;
  status: string;
  team: string;
  projectedPoints: number;
  points: number;
  slotPosition: string;
  isStarter: boolean;
}

export interface MatchupDetail {
  id: string;
  year: number;
  week: number;
  homeTeam: {
    id: number;
    name: string;
    score: number;
    projectedScore: number;
    players: Player[];
  };
  awayTeam: {
    id: number;
    name: string;
    score: number;
    projectedScore: number;
    players: Player[];
  };
}

export interface BoxScorePlayer {
  playerName: string;
  playerPosition: string;
  status: string;
  actualPoints: number;
  projectedPoints: number;
}

export interface GetMatchupDetailResponse {
  data: {
    id: string;
    year: number;
    week: number;
    homeTeamESPNID: number;
    awayTeamESPNID: number;
    homeTeam: {
      id: string;
      name: string;
      score: number;
      projectedScore: number;
      players: Player[];
    };
    awayTeam: {
      id: string;
      name: string;
      score: number;
      projectedScore: number;
      players: Player[];
    };
  };
}

/**
 * Schedule API service
 */
export const scheduleService = {
  getFullSchedule: async (leagueId: number, gameType?: string): Promise<GetMatchupsResponse> => {
    const params = new URLSearchParams();
    if (gameType && gameType !== "all") {
      params.append("gameType", gameType);
    }
    const queryString = params.toString() ? `?${params.toString()}` : "";
    return apiClient.get<GetMatchupsResponse>(`/leagues/${leagueId}/schedules${queryString}`);
  },

  getScheduleByYear: async (leagueId: number, year: number): Promise<GetMatchupsResponse> => {
    return apiClient.get<GetMatchupsResponse>(`/leagues/${leagueId}/schedules?year=${year}`);
  },

  getMatchupById: async (leagueId: number, matchupId: string): Promise<GetMatchupDetailResponse> => {
    return apiClient.get<GetMatchupDetailResponse>(`/leagues/${leagueId}/schedules/${matchupId}`);
  },
};

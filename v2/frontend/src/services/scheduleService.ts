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
  /**
   * Get the full schedule for a specific league
   */
  getFullSchedule: async (leagueId: string, gameType?: string): Promise<GetMatchupsResponse> => {
    const params = new URLSearchParams();
    if (gameType && gameType !== "all") {
      params.append("gameType", gameType);
    }
    const queryString = params.toString() ? `?${params.toString()}` : "";
    return apiClient.get<GetMatchupsResponse>(`/league/${leagueId}/schedules${queryString}`);
  },

  /**
   * Get the schedule for a specific year and league
   */
  getScheduleByYear: async (leagueId: string, year: number): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/league/${leagueId}/schedules?year=${year}`);
  },

  /**
   * Get all completed games for a specific league
   */
  getCompletedGames: async (leagueId: string): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/league/${leagueId}/schedules/completed`);
  },

  /**
   * Get schedule for a specific team in a league
   */
  getTeamSchedule: async (leagueId: string, teamId: number): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/league/${leagueId}/schedules/team/${teamId}`);
  },

  /**
   * Get details for a specific matchup in a league
   */
  getMatchupById: async (
    leagueId: string,
    matchupId: string
  ): Promise<GetMatchupDetailResponse> => {
    return apiClient.get<GetMatchupDetailResponse>(`/league/${leagueId}/schedules/${matchupId}`);
  },
};

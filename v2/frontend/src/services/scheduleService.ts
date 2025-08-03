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
  matchupStatistics: {
    pointDifferential: number;
    accuracyPercentage: number;
    playoffImplications: string;
    winProbability: number;
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
  data: MatchupDetail;
}

/**
 * Schedule API service
 */
export const scheduleService = {
  /**
   * Get the full schedule
   */
  getFullSchedule: async (): Promise<GetMatchupsResponse> => {
    return apiClient.get<GetMatchupsResponse>("/schedules");
  },

  /**
   * Get the schedule for a specific year
   */
  getScheduleByYear: async (year: number): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/schedules?year=${year}`);
  },

  /**
   * Get all completed games
   */
  getCompletedGames: async (): Promise<Game[]> => {
    return apiClient.get<Game[]>("/schedules/completed");
  },

  /**
   * Get schedule for a specific team
   */
  getTeamSchedule: async (teamId: number): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/schedules/team/${teamId}`);
  },

  /**
   * Get details for a specific matchup
   */
  getMatchupById: async (
    matchupId: string
  ): Promise<GetMatchupDetailResponse> => {
    return apiClient.get<GetMatchupDetailResponse>(`/schedules/${matchupId}`);
  },
};

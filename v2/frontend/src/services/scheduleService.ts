import { apiClient } from './apiClient';

// Define interfaces for schedule data
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

export interface Matchup {
  id: string;
  year: number;
  week: number;
  homeTeamId: number;
  awayTeamId: number;
  homeTeamName: string;
  awayTeamName: string;
  homeScore: number;
  awayScore: number;
  homeProjectedScore: number;
  awayProjectedScore: number;
}

/**
 * Schedule API service
 */
export const scheduleService = {
  /**
   * Get the full schedule
   */
  getFullSchedule: async (): Promise<GetScheduleResponse> => {
    return apiClient.get<GetScheduleResponse>('/schedules');
  },

  /**
   * Get the schedule for a specific year
   */
  getScheduleByYear: async (year: number): Promise<Game[][]> => {
    return apiClient.get<Game[][]>(`/schedules?year=${year}`);
  },

  /**
   * Get all completed games
   */
  getCompletedGames: async (): Promise<Game[]> => {
    return apiClient.get<Game[]>('/schedules/completed');
  },

  /**
   * Get schedule for a specific team
   */
  getTeamSchedule: async (teamId: number): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/schedules/team/${teamId}`);
  }
};
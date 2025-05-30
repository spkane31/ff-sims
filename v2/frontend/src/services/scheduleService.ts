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

/**
 * Schedule API service
 */
export const scheduleService = {
  /**
   * Get the full schedule
   */
  getFullSchedule: async (): Promise<Game[][]> => {
    return apiClient.get<Game[][]>('/schedule');
  },

  /**
   * Get the schedule for a specific year
   */
  getScheduleByYear: async (year: number): Promise<Game[][]> => {
    return apiClient.get<Game[][]>(`/schedule?year=${year}`);
  },

  /**
   * Get all completed games
   */
  getCompletedGames: async (): Promise<Game[]> => {
    return apiClient.get<Game[]>('/schedule/completed');
  },

  /**
   * Get schedule for a specific team
   */
  getTeamSchedule: async (teamId: number): Promise<Game[]> => {
    return apiClient.get<Game[]>(`/schedule/team/${teamId}`);
  }
};
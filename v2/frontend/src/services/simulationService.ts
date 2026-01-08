import { apiClient } from './apiClient';

// Define interfaces for simulation data
export interface SimulationResult {
  teamId: number;
  owner: string;
  wins: number;
  losses: number;
  pointsFor: number;
  pointsAgainst: number;
  playoffOdds: number;
  lastPlaceOdds: number;
}

export interface SimulationMetrics {
  simulationCount: number;
  averageRuntime: number;
  lastUpdated: string;
}

/**
 * Simulation API service
 */
export const simulationService = {
  /**
   * Get current simulation results for a specific league
   */
  getSimulationResults: async (leagueId: string): Promise<SimulationResult[]> => {
    return apiClient.get<SimulationResult[]>(`/league/${leagueId}/simulations/results`);
  },

  /**
   * Get simulation metrics for a specific league
   */
  getSimulationMetrics: async (leagueId: string): Promise<SimulationMetrics> => {
    return apiClient.get<SimulationMetrics>(`/league/${leagueId}/simulations/metrics`);
  },

  /**
   * Run a new simulation with custom parameters for a specific league
   */
  runSimulation: async (leagueId: string, params: { iterations?: number, seed?: number }): Promise<SimulationResult[]> => {
    return apiClient.post<SimulationResult[]>(`/league/${leagueId}/simulations/run`, params);
  },
};
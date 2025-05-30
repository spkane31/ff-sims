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
   * Get current simulation results
   */
  getSimulationResults: async (): Promise<SimulationResult[]> => {
    return apiClient.get<SimulationResult[]>('/simulations/results');
  },

  /**
   * Get simulation metrics
   */
  getSimulationMetrics: async (): Promise<SimulationMetrics> => {
    return apiClient.get<SimulationMetrics>('/simulations/metrics');
  },

  /**
   * Run a new simulation with custom parameters
   */
  runSimulation: async (params: { iterations?: number, seed?: number }): Promise<SimulationResult[]> => {
    return apiClient.post<SimulationResult[]>('/simulations/run', params);
  },
};
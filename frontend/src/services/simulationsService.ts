import { apiClient } from "./apiClient";

// Define TypeScript interfaces based on the Go handler response
export interface TeamStats {
  teamId: string;
  teamOwner: string;
  averagePoints: number;
  stdDevPoints: number;
}

export interface GetStatsResponse {
  teamStats: TeamStats[];
}

export interface SimulationRequest {
  iterations: number;
  startWeek: number;
  useActualResults: boolean;
}

export interface SimulationResults {
  playoffOdds: { [teamId: string]: number };
  finalStandings: {
    teamId: string;
    teamOwner: string;
    wins: number;
    losses: number;
    pointsFor: number;
  }[];
  championshipOdds: { [teamId: string]: number };
}

/**
 * Simulations API service
 */
export const simulationsService = {
  /**
   * Get team statistics for simulation
   */
  getStats: async (): Promise<GetStatsResponse> => {
    return apiClient.get<GetStatsResponse>("/simulations/stats");
  },

  /**
   * Run a simulation
   */
  runSimulation: async (
    request: SimulationRequest
  ): Promise<SimulationResults> => {
    return apiClient.post<SimulationResults>("/simulations/run", request);
  },
};

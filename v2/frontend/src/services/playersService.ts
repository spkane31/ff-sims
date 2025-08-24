import { apiClient } from "./apiClient";

// Define TypeScript interfaces for player data
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

export interface GameLogEntry {
  week: number;
  year: number;
  actualPoints: number;
  projectedPoints: number;
  difference: number;
  startedFlag: boolean;
  gameDate: string;
  stats: PlayerStats;
}

export interface GamePerformance {
  points: number;
  year: number;
  week: number;
}

export interface AnnualStatsEntry {
  year: number;
  gamesPlayed: number;
  totalFantasyPoints: number;
  totalProjectedPoints: number;
  avgFantasyPoints: number;
  difference: number;
  bestGame: GamePerformance;
  worstGame: GamePerformance;
  consistencyScore: number; // Standard deviation
  totalStats: PlayerStats;
}

export interface PlayerDetail {
  id: string;
  espnId: string;
  name: string;
  position: string;
  team: string;
  status: string;
  totalFantasyPoints: number;
  totalProjectedPoints: number;
  difference: number;
  gamesPlayed: number;
  avgFantasyPoints: number;
  positionRank: number;
  bestGame: GamePerformance;
  worstGame: GamePerformance;
  consistencyScore: number; // Standard deviation
  totalStats: PlayerStats;
  annualStats: AnnualStatsEntry[];
  gameLog: GameLogEntry[];
}

export interface PlayerSummary {
  id: string;
  espnId: string;
  name: string;
  position: string;
  team: string;
  status: string;
  totalFantasyPoints: number;
  totalProjectedPoints: number;
  difference: number;
  gamesPlayed: number;
  avgFantasyPoints: number;
  positionRank: number;
  totalStats: PlayerStats;
}

export interface GetPlayersResponse {
  players: PlayerSummary[];
  total: number;
  page: number;
  limit: number;
}

export interface PlayerGameLog {
  week: number;
  year: number;
  opponent: string;
  points: number;
  projectedPoints: number;
  status: string;
  gameStats: PlayerStats;
}

export interface PlayerSeasonStats {
  year: number;
  gamesPlayed: number;
  totalFantasyPoints: number;
  totalProjectedPoints: number;
  avgFantasyPoints: number;
  positionRank: number;
  stats: PlayerStats;
}

/**
 * Players API service
 */
export const playersService = {
  /**
   * Get all players with optional filtering and pagination
   */
  getPlayers: async (params?: {
    position?: string;
    year?: string;
    rank?: 'fantasy_points' | 'avg_points' | 'projected_points' | 'games_played' | 'vs_projection';
    page?: number;
    limit?: number;
  }): Promise<GetPlayersResponse> => {
    const queryParams = new URLSearchParams();

    if (params?.position) queryParams.append("position", params.position);
    if (params?.year) queryParams.append("year", params.year);
    if (params?.rank) queryParams.append("rank", params.rank);
    if (params?.page) queryParams.append("page", params.page.toString());
    if (params?.limit) queryParams.append("limit", params.limit.toString());

    const endpoint = `/players${
      queryParams.toString() ? `?${queryParams.toString()}` : ""
    }`;

    return apiClient.get<GetPlayersResponse>(endpoint);
  },

  /**
   * Get a specific player by ID with detailed statistics
   */
  getPlayerDetail: async (id: string | number, year?: string): Promise<PlayerDetail> => {
    const queryParams = new URLSearchParams();
    if (year) queryParams.append("year", year);

    const endpoint = `/players/${id}${
      queryParams.toString() ? `?${queryParams.toString()}` : ""
    }`;

    return apiClient.get<PlayerDetail>(endpoint);
  },

  /**
   * Get player statistics for a specific season
   */
  getPlayerStats: async (
    playerId: string | number,
    params?: {
      week?: string | number;
      season?: string | number;
    }
  ): Promise<PlayerSeasonStats> => {
    const queryParams = new URLSearchParams();
    if (params?.week) queryParams.append("week", params.week.toString());
    if (params?.season) queryParams.append("season", params.season.toString());

    const endpoint = `/players/${playerId}/stats${
      queryParams.toString() ? `?${queryParams.toString()}` : ""
    }`;

    return apiClient.get<PlayerSeasonStats>(endpoint);
  },

  /**
   * Get player game log for a specific season
   */
  getPlayerGameLog: async (
    playerId: string | number,
    year?: string | number
  ): Promise<PlayerGameLog[]> => {
    const queryParams = new URLSearchParams();
    if (year) queryParams.append("year", year.toString());

    const endpoint = `/players/${playerId}/gamelog${
      queryParams.toString() ? `?${queryParams.toString()}` : ""
    }`;

    return apiClient.get<PlayerGameLog[]>(endpoint);
  },

  /**
   * Get players by position
   */
  getPlayersByPosition: async (
    position: string,
    year?: string | number
  ): Promise<PlayerSummary[]> => {
    const queryParams = new URLSearchParams();
    queryParams.append("position", position);
    if (year) queryParams.append("year", year.toString());

    const endpoint = `/players?${queryParams.toString()}`;
    
    const response = await apiClient.get<GetPlayersResponse>(endpoint);
    return response.players;
  },

  /**
   * Get top players by fantasy points
   */
  getTopPlayers: async (
    limit: number = 50,
    position?: string,
    year?: string | number
  ): Promise<PlayerSummary[]> => {
    const queryParams = new URLSearchParams();
    queryParams.append("limit", limit.toString());
    if (position) queryParams.append("position", position);
    if (year) queryParams.append("year", year.toString());

    const endpoint = `/players?${queryParams.toString()}`;
    
    const response = await apiClient.get<GetPlayersResponse>(endpoint);
    return response.players;
  },

  /**
   * Search players by name
   */
  searchPlayers: async (
    query: string,
    limit: number = 20
  ): Promise<PlayerSummary[]> => {
    const queryParams = new URLSearchParams();
    queryParams.append("search", query);
    queryParams.append("limit", limit.toString());

    const endpoint = `/players/search?${queryParams.toString()}`;
    
    return apiClient.get<PlayerSummary[]>(endpoint);
  },

  /**
   * Get player rankings for a specific position
   */
  getPlayerRankings: async (
    position: string,
    year?: string | number
  ): Promise<PlayerSummary[]> => {
    const queryParams = new URLSearchParams();
    queryParams.append("position", position);
    if (year) queryParams.append("year", year.toString());

    const endpoint = `/players/rankings?${queryParams.toString()}`;
    
    return apiClient.get<PlayerSummary[]>(endpoint);
  },
};

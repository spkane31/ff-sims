import { apiClient } from "./apiClient";

export interface DraftProjection {
  projected_points: number | null;
  projection_actual_points: number | null;
}

export interface DraftRewindPick extends DraftProjection {
  id: number;
  player_id: number;
  player: string;
  position: string;
  team_id: number;
  round: number;
  pick: number;
  observed_weeks: number;
  projection_weeks: number;
  score_status: "ranked" | "no_scores" | "conflicting_scores";
  actual_points: number | null;
  round_rank: number | null;
  overall_rank: number | null;
}

export interface DraftRewindTeam extends DraftProjection {
  id: number;
  name: string;
  total_picks: number;
  ranked_picks: number;
  projected_picks: number;
  actual_points: number | null;
}

export interface DraftRewindReport {
  year: number;
  through_week: number;
  ranked_picks: number;
  total_picks: number;
  picks: DraftRewindPick[];
  teams: DraftRewindTeam[];
}

export const draftRewindService = {
  getSeasons: (leagueId: number) =>
    apiClient.get<{ years: number[] }>(`/leagues/${leagueId}/draft/seasons`),
  getReport: (leagueId: number, year: number) =>
    apiClient.get<DraftRewindReport>(`/leagues/${leagueId}/draft/${year}`),
};

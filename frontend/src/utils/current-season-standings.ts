export interface CurrentSeasonStandingInput {
  team_id: number;
  espn_id: string;
  owner: string;
  team_name: string;
  record: {
    wins: number;
    losses: number;
    ties: number;
  };
  points: {
    scored: number;
    against: number;
  };
  expected_wins?: number;
  expected_losses?: number;
  win_luck?: number;
}

export interface CurrentSeasonStandingTeam {
  id: string;
  espnId: string;
  name: string;
  owner: string;
  record: {
    wins: number;
    losses: number;
    ties: number;
  };
  playoffRecord: {
    wins: number;
    losses: number;
    ties: number;
  };
  points: {
    scored: number;
    against: number;
  };
  expectedWins?: {
    expectedWins: number;
    expectedLosses: number;
    winLuck: number;
    seasonsPlayed: number;
  };
  rank: number;
  playoffChance: number;
}

export function toSeasonStandingTeam(
  standing: CurrentSeasonStandingInput,
  rank: number,
): CurrentSeasonStandingTeam {
  const expectedWins =
    standing.expected_wins !== undefined && standing.expected_losses !== undefined
      ? {
          expectedWins: standing.expected_wins,
          expectedLosses: standing.expected_losses,
          winLuck: standing.win_luck ?? 0,
          seasonsPlayed: 1,
        }
      : undefined;

  return {
    id: String(standing.team_id),
    espnId: standing.espn_id,
    name: standing.team_name,
    owner: standing.owner,
    record: standing.record,
    playoffRecord: { wins: 0, losses: 0, ties: 0 },
    points: standing.points,
    expectedWins,
    rank,
    playoffChance: 0,
  };
}

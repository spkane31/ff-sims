import type { Matchup as APIMatchup } from "../types/models";
import type { Schedule } from "../types/simulation";

// Schedule indices are calendar weeks: the simulator uses index + 1 for
// outcomes and selection keys. Missing weeks stay empty, never fabricated.
export function buildSimulationSchedule(
  matchups: APIMatchup[],
  year: number
): Schedule {
  const yearMatchups = matchups.filter((matchup) => matchup.year === year);
  const lastWeek = yearMatchups.reduce(
    (last, matchup) => Math.max(last, matchup.week),
    0
  );
  const schedule: Schedule = Array.from({ length: lastWeek }, () => []);

  for (const matchup of yearMatchups) {
    schedule[matchup.week - 1].push({
      homeTeamName: matchup.homeTeamName,
      awayTeamName: matchup.awayTeamName,
      homeTeamESPNID: matchup.homeTeamESPNID,
      awayTeamESPNID: matchup.awayTeamESPNID,
      homeTeamFinalScore: matchup.homeScore,
      awayTeamFinalScore: matchup.awayScore,
      completed: matchup.completed,
      week: matchup.week,
      gameType: matchup.gameType,
    });
  }

  return schedule;
}

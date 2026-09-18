import { Simulator } from "./simulator";
import type { Matchup, Schedule } from "../types/simulation";

export interface HomepageScheduleMatchup {
  year: number;
  week: number;
  homeTeamName: string;
  awayTeamName: string;
  homeTeamESPNID: number;
  awayTeamESPNID: number;
  homeScore: number;
  awayScore: number;
  completed: boolean;
  gameType: string;
}

// calculateHomepagePlayoffOdds runs the same Monte Carlo model used by the
// simulations page, starting with the first incomplete regular-season week.
export function calculateHomepagePlayoffOdds(
  matchups: HomepageScheduleMatchup[],
  year: number,
  iterations: number,
): Map<number, number> {
  const matchupsByWeek = new Map<number, Matchup[]>();

  for (const matchup of matchups) {
    if (matchup.year !== year || matchup.gameType !== "NONE") {
      continue;
    }

    const week = matchupsByWeek.get(matchup.week) ?? [];
    week.push({
      homeTeamName: matchup.homeTeamName,
      awayTeamName: matchup.awayTeamName,
      homeTeamESPNID: matchup.homeTeamESPNID,
      awayTeamESPNID: matchup.awayTeamESPNID,
      homeTeamFinalScore: matchup.homeScore,
      awayTeamFinalScore: matchup.awayScore,
      completed: matchup.completed,
      gameType: matchup.gameType,
      week: matchup.week,
    });
    matchupsByWeek.set(matchup.week, week);
  }

  const schedule: Schedule = Array.from(matchupsByWeek.entries())
    .sort(([firstWeek], [secondWeek]) => firstWeek - secondWeek)
    .map(([, week]) => week);
  if (schedule.length === 0) {
    return new Map();
  }

  const firstIncompleteWeek = schedule.findIndex((week) =>
    week.some((matchup) => !matchup.completed),
  );
  const startWeek =
    firstIncompleteWeek === -1 ? schedule.length + 1 : firstIncompleteWeek + 1;
  const simulator = new Simulator(schedule, startWeek);

  for (let iteration = 0; iteration < iterations; iteration++) {
    simulator.step();
  }

  return new Map(
    simulator
      .getTeamScoringData()
      .map((team) => [team.id, team.playoffOdds]),
  );
}

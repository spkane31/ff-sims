import assert from "node:assert/strict";
import test from "node:test";
import { buildSimulationSchedule } from "./simulation-schedule";
import { Simulator } from "./simulator";
import type { Matchup } from "../types/models";

function matchup(
  week: number,
  awayTeamESPNID: number,
  overrides: Partial<Matchup> = {}
): Matchup {
  return {
    id: week * 100 + awayTeamESPNID,
    createdAt: "",
    updatedAt: "",
    leagueId: 1,
    season: 2026,
    homeTeamId: 1,
    awayTeamId: awayTeamESPNID,
    isPlayoff: false,
    year: 2026,
    week,
    homeTeamESPNID: 1,
    awayTeamESPNID,
    homeTeamName: "Home",
    awayTeamName: `Team ${awayTeamESPNID}`,
    homeScore: 0,
    awayScore: 0,
    homeProjectedScore: 0,
    awayProjectedScore: 0,
    completed: false,
    gameType: "NONE",
    ...overrides,
  };
}

test("does not invent opponents for weeks missing from the schedule API", () => {
  const schedule = buildSimulationSchedule([matchup(1, 2), matchup(2, 3)], 2026);
  assert.equal(schedule.length, 2);
  assert.deepEqual(schedule.map((week) => week[0].awayTeamESPNID), [2, 3]);
});

test("keeps real future opponents, week numbers, and selection keys through simulation", () => {
  const schedule = buildSimulationSchedule(
    [
      matchup(4, 4),
      matchup(1, 2, { completed: true, homeScore: 110, awayScore: 90 }),
      matchup(1, 4, {
        homeTeamESPNID: 3,
        completed: true,
        homeScore: 100,
        awayScore: 80,
      }),
      matchup(3, 3),
      matchup(3, 99, { year: 2025 }),
    ],
    2026
  );
  assert.deepEqual(schedule[1], []);
  assert.equal(schedule[2][0].awayTeamESPNID, 3);
  assert.equal(schedule[3][0].awayTeamESPNID, 4);

  const simulator = new Simulator(schedule, 3);
  simulator.step();
  const outcomes = simulator.iterations[0].matchupOutcomes;
  assert.deepEqual(
    outcomes.map(({ week, awayTeamId }) => [week, awayTeamId]),
    [[1, 2], [1, 4], [3, 3], [4, 4]]
  );
  const selected = outcomes.find((outcome) => outcome.week === 4)!;
  assert.equal(
    simulator.getFilteredTeamScoringData(new Map([
      ["4-1-4", selected.winnerId],
    ])).matchingCount,
    1
  );
});

test("uses the API completion flag for a matchup with live scores", () => {
  const schedule = buildSimulationSchedule(
    [matchup(1, 2, { completed: false, homeScore: 50, awayScore: 40 })],
    2026
  );
  assert.equal(schedule[0][0].completed, false);
});

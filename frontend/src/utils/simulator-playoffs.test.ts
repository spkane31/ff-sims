import assert from "node:assert/strict";
import test from "node:test";
import { Simulator } from "./simulator";
import type { Schedule } from "../types/simulation";

function randomForGame(firstU: number, firstV: number, secondU: number, secondV: number): number[] {
  return [0.5, 0.5, firstU, firstV, 0.5, 0.5, secondU, secondV, 0.5, 0.5];
}

test("four-team bracket totals both championship weeks", () => {
  const schedule: Schedule = [[
    {
      homeTeamName: "Team 1", awayTeamName: "Team 4",
      homeTeamESPNID: 1, awayTeamESPNID: 4,
      homeTeamFinalScore: 110, awayTeamFinalScore: 90,
      completed: true, gameType: "NONE", week: 1,
    },
    {
      homeTeamName: "Team 2", awayTeamName: "Team 3",
      homeTeamESPNID: 2, awayTeamESPNID: 3,
      homeTeamFinalScore: 105, awayTeamFinalScore: 95,
      completed: true, gameType: "NONE", week: 1,
    },
  ]];
  const simulator = new Simulator(schedule, 2, { teamCount: 4, roundWeeks: [1, 2] });
  for (const teamId of [1, 2, 3, 4]) {
    simulator.teamStats.set(teamId, { average: 100, std_dev: 10 });
  }
  simulator.leagueStats = { mean: 100, stdDev: 0 };

  const sequence = [
    ...randomForGame(0.5, 0.001, 0.5, 0.5), // 1 beats 4
    ...randomForGame(0.5, 0.001, 0.5, 0.5), // 2 beats 3
    ...randomForGame(0.5, 0.001, 0.5, 0.5), // 1 leads 2 after final week one
    ...randomForGame(0.5, 0.5, 0.1, 0.001), // 2 overcomes the deficit in week two
  ];
  const originalRandom = Math.random;
  Math.random = () => sequence.shift() ?? 0.5;
  try {
    simulator.step();
  } finally {
    Math.random = originalRandom;
  }

  const results = simulator.iterations[0].teamResults;
  assert.equal(results.get(1)?.playoffResult, 2);
  assert.equal(results.get(2)?.playoffResult, 1);
  assert.equal(results.get(3)?.playoffResult, 3);
  assert.equal(results.get(4)?.playoffResult, 4);
});

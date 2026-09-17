import assert from "node:assert/strict";
import test from "node:test";
// Node's test runner requires the TypeScript extension at runtime.
// @ts-expect-error TypeScript source imports are not enabled for production code.
import { toSeasonStandingTeam } from "./current-season-standings.ts";

test("maps API expected wins into the homepage standings row", () => {
  const team = toSeasonStandingTeam(
    {
      team_id: 7,
      espn_id: "42",
      owner: "Alex",
      team_name: "The Test Team",
      record: { wins: 4, losses: 2, ties: 0 },
      points: { scored: 720.5, against: 690.25 },
      expected_wins: 4.67,
      expected_losses: 1.33,
      win_luck: -0.67,
    },
    1,
  );

  assert.deepEqual(team.expectedWins, {
    expectedWins: 4.67,
    expectedLosses: 1.33,
    winLuck: -0.67,
    seasonsPlayed: 1,
  });
});

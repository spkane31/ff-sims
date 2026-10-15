import assert from "node:assert/strict";
import test from "node:test";
import { calculateHomepagePlayoffOdds } from "./homepage-playoff-odds";

test("returns deterministic playoff odds from a completed regular season", () => {
  const odds = calculateHomepagePlayoffOdds(
    [
      matchup(1, 2, 110, 90),
      matchup(3, 4, 109, 89),
      matchup(5, 6, 108, 88),
      matchup(7, 8, 107, 87),
    ],
    2026,
    10,
    { teamCount: 6, roundWeeks: [1, 1, 1] },
  );

  assert.equal(odds.size, 8);
  assert.equal([...odds.values()].reduce((total, value) => total + value, 0), 6);
  assert.deepEqual([...odds.values()].sort(), [0, 0, 1, 1, 1, 1, 1, 1]);
});

test("uses a four-team playoff cutoff when the league has four qualifiers", () => {
  const odds = calculateHomepagePlayoffOdds(
    [
      matchup(1, 2, 110, 90),
      matchup(3, 4, 109, 89),
      matchup(5, 6, 108, 88),
      matchup(7, 8, 107, 87),
    ],
    2026,
    10,
    { teamCount: 4, roundWeeks: [1, 2] },
  );

  assert.equal([...odds.values()].reduce((total, value) => total + value, 0), 4);
  assert.deepEqual([...odds.values()].sort(), [0, 0, 0, 0, 1, 1, 1, 1]);
});

function matchup(
  homeTeamESPNID: number,
  awayTeamESPNID: number,
  homeScore: number,
  awayScore: number,
) {
  return {
    year: 2026,
    week: 1,
    homeTeamName: `Team ${homeTeamESPNID}`,
    awayTeamName: `Team ${awayTeamESPNID}`,
    homeTeamESPNID,
    awayTeamESPNID,
    homeScore,
    awayScore,
    completed: true,
    gameType: "NONE",
  };
}

import assert from "node:assert/strict";
import test from "node:test";
import { Simulator } from "./simulator";
import type { Matchup, Schedule } from "../types/simulation";

const TEAM_COUNT = 10;
const WEEKS = 14;
const START_WEEK = 8;
const ITERATIONS = 4000;

/**
 * Deterministic Math.random so a swing is reproducible across runs. Returns the
 * restore function.
 */
function stubRandom(seed: number): () => void {
  const original = Math.random;
  let state = seed >>> 0;
  Math.random = () => {
    state = (state * 1664525 + 1013904223) >>> 0;
    return state / 4294967296;
  };
  return () => {
    Math.random = original;
  };
}

/** Circle-method round robin over TEAM_COUNT teams, team 10 fixed. */
function pairingsForWeek(week: number): Array<[number, number]> {
  const rotating = [1, 2, 3, 4, 5, 6, 7, 8, 9];
  const r = (week - 1) % rotating.length;
  const rot = rotating.slice(r).concat(rotating.slice(0, r));
  const pairs: Array<[number, number]> = [[TEAM_COUNT, rot[0]]];
  for (let i = 1; i <= 4; i++) {
    pairs.push([rot[i], rot[9 - i]]);
  }
  return pairs;
}

/** Distinct per-team mean with a repeatable wobble, so std devs differ too. */
function completedScore(teamId: number, week: number): number {
  return 90 + teamId * 2.5 + (((teamId * 7 + week * 13) % 23) - 11) * 2;
}

function buildSchedule(): Schedule {
  const schedule: Schedule = [];
  for (let week = 1; week <= WEEKS; week++) {
    const completed = week < START_WEEK;
    const matchups: Matchup[] = pairingsForWeek(week).map(([home, away]) => ({
      homeTeamName: `Team ${home}`,
      awayTeamName: `Team ${away}`,
      homeTeamESPNID: home,
      awayTeamESPNID: away,
      homeTeamFinalScore: completed ? completedScore(home, week) : 0,
      awayTeamFinalScore: completed ? completedScore(away, week) : 0,
      completed,
      gameType: "NONE",
      week,
    }));
    schedule.push(matchups);
  }
  return schedule;
}

function runSimulator(seed: number): Simulator {
  const restore = stubRandom(seed);
  try {
    const sim = new Simulator(buildSchedule(), START_WEEK);
    for (let i = 0; i < ITERATIONS; i++) {
      sim.step();
    }
    return sim;
  } finally {
    restore();
  }
}

test("ranks a game by the odds it moves for every team, not just the two playing", () => {
  const sim = runSimulator(7);
  const games = sim.getMostImportantMatchups();

  assert.ok(games.length > 0, "expected at least one big game");

  const top = games[0];
  const participantSwing = top.teams
    .filter(
      (team) => team.teamId === top.homeTeamId || team.teamId === top.awayTeamId
    )
    .reduce((sum, team) => sum + team.swing, 0);
  const allTeamSwing = top.teams.reduce((sum, team) => sum + team.swing, 0);

  assert.equal(top.teams.length, TEAM_COUNT);
  assert.ok(
    Math.abs(top.totalSwing - allTeamSwing) < 1e-9,
    `totalSwing ${top.totalSwing} should be the sum over all teams ${allTeamSwing}`
  );
  assert.ok(
    top.totalSwing > participantSwing + 0.01,
    `non-participants should contribute: total ${top.totalSwing} vs participants ${participantSwing}`
  );
});

test("each team's swing is its absolute change away from the baseline under both outcomes", () => {
  const sim = runSimulator(11);
  const games = sim.getMostImportantMatchups();
  assert.ok(games.length > 0, "expected at least one big game");

  for (const game of games) {
    for (const team of game.teams) {
      const viaBaseline =
        Math.abs(team.homeWinPlayoffOdds - team.baselinePlayoffOdds) +
        Math.abs(team.awayWinPlayoffOdds - team.baselinePlayoffOdds) +
        Math.abs(team.homeWinLastPlaceOdds - team.baselineLastPlaceOdds) +
        Math.abs(team.awayWinLastPlaceOdds - team.baselineLastPlaceOdds);

      assert.ok(
        Math.abs(team.swing - viaBaseline) < 1e-9,
        `${game.week} ${team.teamName}: swing ${team.swing} vs baseline deltas ${viaBaseline}`
      );

      // Baseline is the convex combination of the two outcomes, so the two
      // formulations have to agree. They stop agreeing if the home and away
      // iteration counts ever fail to cover the whole baseline.
      const viaOutcomes =
        Math.abs(team.homeWinPlayoffOdds - team.awayWinPlayoffOdds) +
        Math.abs(team.homeWinLastPlaceOdds - team.awayWinLastPlaceOdds);

      assert.ok(
        Math.abs(team.swing - viaOutcomes) < 1e-9,
        `${game.week} ${team.teamName}: swing ${team.swing} vs outcome delta ${viaOutcomes}`
      );
    }
  }
});

test("only considers games inside the week window", () => {
  const sim = runSimulator(13);

  assert.ok(
    sim.schedule.some((week) => week.some((m) => m.week > START_WEEK + 3)),
    "fixture should have games beyond the default window"
  );

  for (const game of sim.getMostImportantMatchups()) {
    assert.ok(
      game.week >= START_WEEK && game.week <= START_WEEK + 3,
      `week ${game.week} outside the default four-week window`
    );
  }

  for (const game of sim.getMostImportantMatchups(5, 2)) {
    assert.ok(
      game.week >= START_WEEK && game.week <= START_WEEK + 1,
      `week ${game.week} outside the two-week window`
    );
  }
});

test("drops games whose total swing is under the floor", () => {
  const sim = runSimulator(17);
  const swings = sim
    .getMostImportantMatchups(100, 4, 0)
    .map((game) => game.totalSwing);
  assert.ok(swings.length > 2, "fixture should produce several games");

  // Cut at the fixture's own median so neither side of the check is empty.
  const cutoff = swings[Math.floor(swings.length / 2)];
  const expected = swings.filter((swing) => swing >= cutoff);
  assert.ok(
    expected.length > 0 && expected.length < swings.length,
    "cutoff should split the fixture's games"
  );
  assert.deepEqual(
    sim.getMostImportantMatchups(100, 4, cutoff).map((game) => game.totalSwing),
    expected
  );

  // The floor defaults to two percent, well under that median cutoff.
  assert.deepEqual(
    sim.getMostImportantMatchups(100, 4).map((game) => game.totalSwing),
    sim.getMostImportantMatchups(100, 4, 0.02).map((game) => game.totalSwing)
  );
  assert.ok(
    sim.getMostImportantMatchups(100, 4, cutoff).length <
      sim.getMostImportantMatchups(100, 4).length,
    "the default floor should keep more games than the median cutoff"
  );
});

test("returns the five biggest games, ordered by swing", () => {
  const sim = runSimulator(19);

  assert.ok(
    sim.getMostImportantMatchups(100, 4, 0.02).length > 5,
    "fixture should have more than five qualifying games"
  );

  const games = sim.getMostImportantMatchups();
  assert.equal(games.length, 5);
  for (let i = 1; i < games.length; i++) {
    assert.ok(
      games[i - 1].totalSwing >= games[i].totalSwing,
      "games should be ordered by descending swing"
    );
  }
});

import assert from "node:assert/strict";
import test from "node:test";
import { Simulator } from "./simulator";
import type { Matchup, SingleTeamResult } from "../types/simulation";

function matchup(week: number, home: number, away: number, gameType = "NONE"): Matchup {
  return {
    week, homeTeamESPNID: home, awayTeamESPNID: away,
    homeTeamName: `Team ${home}`, awayTeamName: `Team ${away}`,
    homeTeamFinalScore: 0, awayTeamFinalScore: 0, completed: false, gameType,
  };
}

function fixture(): Simulator {
  return new Simulator([
    [matchup(1, 1, 2), matchup(1, 3, 4)],
    [matchup(2, 1, 2), matchup(2, 3, 4), matchup(2, 5, 6, "WINNERS_BRACKET")],
    [matchup(3, 1, 3), matchup(3, 2, 4)],
  ], 2);
}

function addScenario(sim: Simulator, winners: number[], count: number, playoffs: number, last: number) {
  for (let i = 0; i < count; i++) {
    const teamResults = new Map<number, SingleTeamResult>();
    for (const id of sim.getTeamIDs()) {
      const result: SingleTeamResult = {
        id, wins: 0, losses: 0, pointsFor: 0, pointsAgainst: 0,
        madePlayoffs: id === 1 ? i < playoffs : id === 3,
        lastPlace: id === 1 ? i < last : id === 4 && i >= last,
        regularSeasonResult: id, playoffResult: -1,
      };
      teamResults.set(id, result);
      sim.getTeamResults(id)!.addSingleSeasonResults(result);
    }
    // Vary prior/later games and the order of recorded games. Neither should
    // split a scenario for week 2. The playoff game must also be ignored.
    const matchupOutcomes = [
      { week: 1, homeTeamId: 1, awayTeamId: 2, winnerId: i % 2 ? 1 : 2 },
      { week: 2, homeTeamId: 1, awayTeamId: 2, winnerId: winners[0] },
      { week: 2, homeTeamId: 3, awayTeamId: 4, winnerId: winners[1] },
      { week: 2, homeTeamId: 5, awayTeamId: 6, winnerId: i % 2 ? 5 : 6 },
      { week: 3, homeTeamId: 1, awayTeamId: 3, winnerId: i % 2 ? 1 : 3 },
    ];
    sim.iterations.push({ matchupOutcomes: i % 2 ? matchupOutcomes.reverse() : matchupOutcomes, teamResults });
    sim.simulations++;
  }
}

test("weekly leverage conditions on every game together and weights baseline by iterations", () => {
  const sim = fixture();
  addScenario(sim, [1, 3], 10, 9, 1);
  addScenario(sim, [1, 4], 20, 4, 12);
  addScenario(sim, [2, 3], 10, 4, 2);
  addScenario(sim, [2, 4], 10, 1, 8);

  const leverage = sim.getWeeklyLeverage()!;
  assert.equal(leverage.week, 2);
  assert.equal(leverage.scenarioCount, 4);
  assert.equal(leverage.possibleScenarioCount, 4);
  assert.equal(leverage.smallestScenarioCount, 10);
  assert.equal(leverage.teams.length, sim.getTeamIDs().length);
  const team = leverage.teams.find((team) => team.teamId === 1)!;
  assert.deepEqual(team.playoff, { baseline: 18 / 50, min: 0.1, max: 0.9 });
  assert.deepEqual(team.lastPlace, { baseline: 23 / 50, min: 0.1, max: 0.8 });
  // Conditioning on team 1's own win alone would give 13/30, not the
  // 90% ceiling reached when team 3 also wins.
  assert.ok(team.playoff.max > 13 / 30);
  assert.deepEqual(leverage.teams.find((team) => team.teamId === 3)!.playoff,
    { baseline: 1, min: 1, max: 1 });
  assert.deepEqual(leverage.teams.find((team) => team.teamId === 2)!.playoff,
    { baseline: 0, min: 0, max: 0 });
});

test("reports sparse scenario coverage without inventing odds for unseen combinations", () => {
  const sim = fixture();
  addScenario(sim, [1, 3], 1, 1, 0);
  const leverage = sim.getWeeklyLeverage()!;
  assert.equal(leverage.scenarioCount, 1);
  assert.equal(leverage.possibleScenarioCount, 4);
  assert.equal(leverage.smallestScenarioCount, 1);
  assert.deepEqual(leverage.teams.find((team) => team.teamId === 1)!.playoff,
    { baseline: 1, min: 1, max: 1 });
});

test("returns no weekly leverage before a run or without regular-season games in the selected week", () => {
  assert.equal(fixture().getWeeklyLeverage(), null);
  const sim = fixture();
  addScenario(sim, [1, 3], 10, 5, 5);
  sim.startWeek = 4;
  assert.equal(sim.getWeeklyLeverage(), null);
  sim.schedule[1] = [matchup(2, 5, 6, "WINNERS_BRACKET")];
  sim.startWeek = 2;
  assert.equal(sim.getWeeklyLeverage(), null);
});

test("does not treat a missing matchup result as a complete weekly scenario", () => {
  const sim = fixture();
  addScenario(sim, [1, 3], 1, 1, 0);
  sim.iterations[0].matchupOutcomes = sim.iterations[0].matchupOutcomes.filter(
    (outcome) => outcome.week !== 2 || outcome.homeTeamId !== 3
  );
  assert.equal(sim.getWeeklyLeverage(), null);
});

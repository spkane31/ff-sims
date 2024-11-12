import SimulatorV2 from "./simulatorv2";
import { Game, Schedule, TeamStats } from "./simulatorv2";
import { getSchedule, getTeams } from "../db/db";

describe("TeamStats", () => {
  it("should be calculated properly", () => {
    const teamStats = new TeamStats([1, 2, 3, 4, 5]);
    expect(teamStats.average()).toBe(3);
    expect(teamStats.variance()).toBe(2.5);
    expect(teamStats.stdDev()).toBe(1.5811388300841898);
  });
});

describe("SimulatorV2", () => {
  let simulator: SimulatorV2;

  beforeEach(async () => {
    const year = 2024;
    // const teamStats = await getTeams(year);
    const schedule = await getSchedule(year);

    let games: Game[] = [];
    schedule.map((games) => {
      games.map((game) => {
        games.push({
          home_team_id: game.home_team_id,
          away_team_id: game.away_team_id,
          home_team_score: game.home_team_score,
          away_team_score: game.away_team_score,
          completed: game.completed,
          week: game.week,
        });
      });
    });

    simulator = new SimulatorV2(new Schedule(games));
  });

  it("should create a new instance of SimulatorV2", () => {
    expect(simulator).toBeInstanceOf(SimulatorV2);
  });

  it("should calculate team data with sample data", () => {
    const games: Game[] = [
      new Game(1, 2, 24, 21, true, 1),
      new Game(3, 4, 28, 17, true, 1),
      new Game(1, 3, 21, 24, true, 2),
      new Game(2, 4, 17, 28, true, 2),
      new Game(1, 4, 0, 0, false, 3),
      new Game(2, 3, 0, 0, false, 3),
    ];
    const schedule = new Schedule(games);
    const simulator = new SimulatorV2(schedule);

    simulator.simulate(50);

    expect(simulator.simulationResults.length).toBe(50);
    expect(simulator.filterResults.length).toBe(50);

    // Add a filter on a result already known
    simulator.addFilter(1, 1);
    expect(simulator.filterResults.length).toBe(50);

    simulator.addFilter(3, 4);
    const filteredFirst = simulator.filterResults.length;
    expect(filteredFirst).toBeLessThan(50);

    simulator.addFilter(3, 2);
    expect(simulator.filterResults.length).toBeLessThan(50);

    simulator.removeFilter(3, 2);
    expect(simulator.filterResults.length).toBe(filteredFirst);
  });
});

/*
describe("Simulator", () => {
  let simulator: SimulatorV2;

  beforeEach(async () => {
    // get the teamAvgs data
    const year = 2023;
    const teamStats = await getTeams(year);
    const schedule = await getSchedule(year);

    simulator = new Simulator(teamStats, schedule);
  });

  it("should be properly initialized", async () => {
    expect(simulator.schedule).toBeDefined();
    expect(simulator.schedule.length).toBe(14);
    expect(simulator.results).toBeDefined();
    expect(simulator.results.size).toBe(10);
    expect(simulator.leagueStats).toBeDefined();
    expect(simulator.teamStats.size).toBe(10);
    expect(simulator.simulations).toBe(0);
    expect(simulator.getResults()).toBeDefined();
    expect(simulator.getTeamResults(1)).toBeDefined();
    expect(simulator.getTeamStats(1)).toBeDefined();
    expect(simulator.weeks).toBe(14);
    expect(simulator.weeksCompleted).toBe(1);
  });

  // it("should increment the wins count for the specified team", async () => {
  //   const teamId = 1;

  //   simulator.teamWin(teamId);

  //   expect(simulator.results.get(teamId).wins).toBe(1);
  // });

  // it("should not increment the wins count for other teams", async () => {
  //   const teamId = 1;
  //   const otherTeamId = 5;

  //   simulator.teamWin(teamId);
  //   simulator.teamWin(otherTeamId);

  //   expect(simulator.results.get(teamId).wins).toBe(1);
  //   expect(simulator.results.get(otherTeamId).wins).toBe(1);
  // });

  // it("should increase each teams points and games for each step", async () => {
  //   simulator.step();

  //   const teams = simulator.getTeamIDs();
  //   expect(teams.length).toBe(10);

  //   teams.forEach((team) => {
  //     const teamResults = simulator.getTeamResults(team);
  //     expect(teamResults.pointsFor).toBeGreaterThan(0);
  //     expect(teamResults.pointsAgainst).toBeGreaterThan(0);
  //     expect(teamResults.wins + teamResults.losses).toBe(14);
  //   });
  // });

  // it("should increase each teams points and games for each step, two steps", async () => {
  //   simulator.step();
  //   simulator.step();

  //   const teams = simulator.getTeamIDs();
  //   expect(teams.length).toBe(10);

  //   teams.forEach((teamID) => {
  //     const teamResults = simulator.getTeamResults(teamID);
  //     expect(teamResults.pointsFor).toBeGreaterThan(0);
  //     expect(teamResults.pointsAgainst).toBeGreaterThan(0);
  //     expect(teamResults.wins + teamResults.losses).toBe(28);
  //   });

  //   const scoringData = simulator.getTeamScoringData();
  //   expect(scoringData.length).toBe(10);
  // });
});

*/

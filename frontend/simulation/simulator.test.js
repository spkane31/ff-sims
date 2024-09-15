import Simulator from "./simulator";
import { getSchedule, getTeams } from "../db/db";

describe("Simulator", () => {
  let simulator;

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

  it("should increment the wins count for the specified team", async () => {
    const teamId = 1;

    simulator.teamWin(teamId);

    expect(simulator.results.get(teamId).wins).toBe(1);
  });

  it("should not increment the wins count for other teams", async () => {
    const teamId = 1;
    const otherTeamId = 5;

    simulator.teamWin(teamId);
    simulator.teamWin(otherTeamId);

    expect(simulator.results.get(teamId).wins).toBe(1);
    expect(simulator.results.get(otherTeamId).wins).toBe(1);
  });

  it("should increase each teams points and games for each step", async () => {
    simulator.step();

    const teams = simulator.getTeamIDs();
    expect(teams.length).toBe(10);

    teams.forEach((team) => {
      const teamResults = simulator.getTeamResults(team);
      expect(teamResults.pointsFor).toBeGreaterThan(0);
      expect(teamResults.pointsAgainst).toBeGreaterThan(0);
      expect(teamResults.wins + teamResults.losses).toBe(14);
    });
  });

  it("should increase each teams points and games for each step, two steps", async () => {
    simulator.step();
    simulator.step();

    const teams = simulator.getTeamIDs();
    expect(teams.length).toBe(10);

    teams.forEach((teamID) => {
      const teamResults = simulator.getTeamResults(teamID);
      expect(teamResults.pointsFor).toBeGreaterThan(0);
      expect(teamResults.pointsAgainst).toBeGreaterThan(0);
      expect(teamResults.wins + teamResults.losses).toBe(28);
    });

    const scoringData = simulator.getTeamScoringData();
    expect(scoringData.length).toBe(10);
  });
});

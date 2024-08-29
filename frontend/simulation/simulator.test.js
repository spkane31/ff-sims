import Simulator from "./simulator";
import team_to_id from "../data/team_to_id.json";

describe("Simulator", () => {
  let simulator;

  beforeEach(() => {
    simulator = new Simulator();
  });

  it("should be properly initialized", () => {
    expect(simulator.schedule).toBeDefined();
    expect(simulator.schedule.length).toBe(14);
    expect(simulator.results).toBeDefined();
    expect(simulator.results.size).toBe(10);
    expect(simulator.leagueStats).toBeDefined();
    expect(simulator.teamStats.size).toBe(10); // League is included here
    expect(simulator.simulations).toBe(0);
    expect(simulator.getResults()).toBeDefined();
    expect(simulator.getTeamResults("Sean  Kane")).toBeDefined();
    expect(simulator.getTeamStats("Sean  Kane")).toBeDefined();
  });

  it("should increment the wins count for the specified team", () => {
    const teamName = "Sean  Kane";
    const teamId = team_to_id[teamName];

    simulator.teamWin(teamName);

    expect(simulator.results.get(teamId).wins).toBe(1);
  });

  it("should not increment the wins count for other teams", () => {
    const teamName = "Sean  Kane";
    const otherTeamName = "Connor Brand";
    const teamId = team_to_id[teamName];
    const otherTeamId = team_to_id[otherTeamName];

    simulator.teamWin(teamName);
    simulator.teamWin(otherTeamName);

    expect(simulator.results.get(teamId).wins).toBe(1);
    expect(simulator.results.get(otherTeamId).wins).toBe(1);
  });

  it("should increase each teams points and games for each step", () => {
    simulator.step();

    const teams = simulator.getTeams();
    expect(teams.length).toBe(10);

    teams.forEach((team) => {
      const teamResults = simulator.getTeamResults(team);
      expect(teamResults.pointsFor).toBeGreaterThan(0);
      expect(teamResults.pointsAgainst).toBeGreaterThan(0);
      expect(teamResults.wins + teamResults.losses).toBe(14);
    });
  });

  it("should increase each teams points and games for each step, two steps", () => {
    simulator.step();
    simulator.step();

    const teams = simulator.getTeams();
    expect(teams.length).toBe(10);

    teams.forEach((team) => {
      const teamResults = simulator.getTeamResults(team);
      expect(teamResults.pointsFor).toBeGreaterThan(0);
      expect(teamResults.pointsAgainst).toBeGreaterThan(0);
      expect(teamResults.wins + teamResults.losses).toBe(28);
    });
  });
});

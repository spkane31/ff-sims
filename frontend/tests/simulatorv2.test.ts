import {
  SimulatorV2,
  TeamStats,
  Game,
  Schedule,
  Filter,
  SimulationResult,
  filterMatches,
} from "../simulation/simulatorv2";
// TODO seankane: hard code a real-ish schedule so that this can be tested consistently
import { getSchedule } from "../db/dbts";

describe("TeamStats", () => {
  it("should be calculated properly", () => {
    const teamStats = new TeamStats([1, 2, 3, 4, 5]);
    expect(teamStats.average()).toBe(3);
    expect(teamStats.variance()).toBe(2.5);
    expect(teamStats.stdDev()).toBe(1.5811388300841898);
  });
});

describe("filterMatches", () => {
  it("should properly filter out games", () => {
    const f = new Filter(1, 2);
    const f1 = new Filter(1, 1);
    const fBad = new Filter(2, 5);
    const sr = new SimulationResult([new Game(1, 2, 24, 21, true, 1)]);
    expect(filterMatches(f, sr)).toBe(false);
    expect(filterMatches(f1, sr)).toBe(true);
    expect(filterMatches(fBad, sr)).toBe(false);
  });

  it("should properly filter out SimulationResults with multiple Games", () => {
    const f = new Filter(1, 2);
    const f1 = new Filter(1, 1);
    const f2 = new Filter(3, 1);
    const f3 = new Filter(3, 2);
    const sr = new SimulationResult([
      new Game(1, 2, 24, 21, true, 1),
      new Game(1, 2, 20, 21, true, 2),
      new Game(1, 2, 25, 22, true, 3),
      new Game(1, 2, 17, 24, true, 4),
    ]);
    expect(filterMatches(f, sr)).toBe(false);
    expect(filterMatches(f1, sr)).toBe(true);
    expect(filterMatches(f2, sr)).toBe(true);
    expect(filterMatches(f3, sr)).toBe(false);
  });
});

describe("SimulatorV2", () => {
  let simulator: SimulatorV2;

  beforeEach(async () => {
    const year = 2024;
    const schedule = await getSchedule(year);

    let games: Game[] = [];
    schedule.map((allGames) => {
      allGames.map((game) => {
        games.push(
          new Game(
            game.home_team_espn_id,
            game.away_team_espn_id,
            game.home_team_final_score,
            game.away_team_final_score,
            game.completed,
            game.week
          )
        );
      });
    });

    simulator = new SimulatorV2(new Schedule(games));
    expect(simulator).toBeDefined();
    expect(simulator.numSimulations).toBe(1000);
  });

  it("should create a new instance of SimulatorV2", () => {
    expect(simulator).toBeInstanceOf(SimulatorV2);
  });

  it("should be able to add and remove filters", () => {
    expect(simulator).toBeInstanceOf(SimulatorV2);

    simulator.addFilter(1, 2);
    expect(simulator.filters.length).toBe(1);
    simulator.addFilter(2, 3);
    expect(simulator.filters.length).toBe(2);
    simulator.addFilter(2, 5);
    expect(simulator.filters.length).toBe(3);
    simulator.addFilter(2, 3);
    expect(simulator.filters.length).toBe(3);

    simulator.removeFilter(2, 3);
    expect(simulator.filters.length).toBe(2);
    simulator.removeFilter(2, 4);
    expect(simulator.filters.length).toBe(2);
    simulator.removeFilter(1, 2);
    expect(simulator.filters.length).toBe(1);
    simulator.removeFilter(2, 5);
    expect(simulator.filters.length).toBe(0);
  });

  it("should simulate with unique scores", () => {
    const games: Game[] = [
      new Game(1, 2, 24, 21, true, 1),
      new Game(1, 2, 25, 20, true, 2),
      new Game(1, 2, 0, 0, false, 3),
    ];

    const n = 2;
    const schedule = new Schedule(games);
    const simulator = new SimulatorV2(schedule, n);

    simulator.simulate();

    expect(simulator.simulationResults.length).toBe(2);
    expect(
      simulator.simulationResults[0].games[2].home_team_score
    ).toBeGreaterThan(0);
    expect(
      simulator.simulationResults[0].games[2].away_team_score
    ).toBeGreaterThan(0);
    expect(
      simulator.simulationResults[1].games[2].home_team_score
    ).toBeGreaterThan(0);
    expect(
      simulator.simulationResults[1].games[2].away_team_score
    ).toBeGreaterThan(0);

    const first = simulator.simulationResults[0];
    const second = simulator.simulationResults[1];

    // Assert the generated scores are different
    expect(first.games[2].home_team_score).not.toBe(
      second.games[0].home_team_score
    );
    expect(first.games[2].away_team_score).not.toBe(
      second.games[0].away_team_score
    );

    // Get the winner in simulation 1
    const winnerId = simulator.simulationResults[0].games[2].homeWins() ? 1 : 2;
    const loserId = winnerId === 1 ? 2 : 1;
    const f = new Filter(3, loserId);
    expect(filterMatches(f, first)).toBe(false);

    simulator.addFilter(3, loserId);

    expect(simulator.filteredResults.length).toBeLessThan(n);
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
    const n = 500;
    const schedule = new Schedule(games);
    const simulator = new SimulatorV2(schedule, n);

    simulator.simulate(n);

    expect(simulator.simulationResults.length).toBe(n);
    expect(simulator.filteredResults.length).toBe(n);

    // Add a filter on a result already known
    simulator.addFilter(1, 1);
    expect(simulator.filteredResults.length).toBe(n);

    simulator.addFilter(3, 4);
    const filteredFirst = simulator.filteredResults.length;
    expect(filteredFirst).toBeLessThan(n);

    // Figure out who won the fifth game in the first simulation and add a filter for the opposite
    const fifthGame = simulator.simulationResults[0].games[5];
    const loser =
      fifthGame.home_team_score < fifthGame.away_team_score
        ? fifthGame.home_team_id
        : fifthGame.away_team_id;

    simulator.addFilter(3, loser);
    expect(simulator.filteredResults.length).toBeLessThan(n);

    simulator.removeFilter(3, loser);
    expect(simulator.filteredResults.length).toBe(filteredFirst);
  });

  it("should handle real data", () => {
    simulator.simulate();
    expect(simulator.simulationResults.length).toBe(1000);
    expect(simulator.filteredResults.length).toBe(1000);

    let week = 0;
    let winnerId = 0;
    for (let i = 0; i < simulator.simulationResults[0].games.length; i++) {
      if (!simulator.simulationResults[0].games[i].completed) {
        continue;
      }

      week = simulator.simulationResults[0].games[i].week;
      winnerId = simulator.simulationResults[0].games[i].home_team_id;
      simulator.addFilter(week, winnerId);

      break;
    }

    expect(simulator.filteredResults.length).toBeLessThan(1000);

    simulator.removeFilter(week, winnerId);

    expect(simulator.filteredResults.length).toBe(1000);

    // Validate that all teams have a greater than 0 chance of making the playoffs
    const teams = new Set<number>();
    simulator.filteredResults.forEach((result) => {
      result.games.forEach((game) => {
        if (game.completed) {
          teams.add(game.home_team_id);
          teams.add(game.away_team_id);
        }
      });
    });

    expect(teams.size).toBe(10);

    let totalLastPlace = 0;
    let totalPlayoffs = 0;
    teams.forEach((team) => {
      const odds = simulator.playoffOdds(team);
      expect(odds).toBeGreaterThanOrEqual(0);
      expect(odds).toBeLessThanOrEqual(1);
      totalPlayoffs += odds;

      const lastPlaceOdds = simulator.lastPlaceOdds(team);
      expect(lastPlaceOdds).toBeGreaterThanOrEqual(0);
      expect(lastPlaceOdds).toBeLessThanOrEqual(1);
      totalLastPlace += lastPlaceOdds;
    });
    expect(totalLastPlace).toBe(1);
    expect(totalPlayoffs).toBe(6);
  });
});

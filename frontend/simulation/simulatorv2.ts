import { normalDistribution } from "../utils/math";

export default class SimulatorV2 {
  schedule: Schedule;
  numSimulations: number;
  simulationResults: SimulationResult[];
  filteredResults: SimulationResult[];
  filters: Filter[];
  teamStats: Map<number, TeamStats>;
  leagueStats: TeamStats;
  constructor(schedule: Schedule);
  constructor(schedule: Schedule, numSimulations: number);
  constructor(
    schedule: Schedule,
    numSimulations?: number,
    SimulationResults?: SimulationResult[]
  ) {
    this.schedule = schedule;
    this.numSimulations = numSimulations === undefined ? 1000 : numSimulations;
    this.simulationResults =
      SimulationResults === undefined ? [] : SimulationResults;
    this.filteredResults = [];
    this.teamStats = new Map<number, TeamStats>();
    this.leagueStats = new TeamStats([]);
    this.filters = [];

    this.generateTeamStats();
  }

  generateTeamStats(): void {
    this.schedule.games.map((game) => {
      if (!this.teamStats.has(game.home_team_id)) {
        this.teamStats.set(game.home_team_id, new TeamStats([]));
      }
      if (!this.teamStats.has(game.away_team_id)) {
        this.teamStats.set(game.away_team_id, new TeamStats([]));
      }

      this.teamStats
        .get(game.home_team_id)
        ?.pointsScored.push(game.home_team_score);
      this.teamStats
        .get(game.away_team_id)
        ?.pointsScored.push(game.away_team_score);

      this.leagueStats.pointsScored.push(game.home_team_score);
      this.leagueStats.pointsScored.push(game.away_team_score);
    });
  }

  simulate(steps?: number): void {
    let n: number = steps === undefined ? this.numSimulations : steps;
    for (let i = 0; i < n; i++) {
      let results: SimulationResult = new SimulationResult(this.schedule.games);
      results.simulate();
      this.simulationResults.push(results);
    }
  }

  addFilter(week: number, winnerId: number): void {
    this.filters.push(new Filter(week, winnerId));
    this.filter();
  }

  removeFilter(week: number, winnerId: number): void {
    this.filters = this.filters.filter(
      (filter) => filter.week !== week && filter.winnerId !== winnerId
    );
    this.filter();
  }

  clearFilters(): void {
    this.filters = [];
    this.filteredResults = this.simulationResults;
  }

  filter(): void {
    this.filters.map((filter) => {
      this.filteredResults = this.filterResults(filter.week, filter.winnerId);
    });
  }

  filterResults(week: number, winnerId: number): SimulationResult[] {
    return this.simulationResults.filter((result) => {
      for (let i: number = 0; i < result.games.length; i++) {
        if (result.games[i].week === week) {
          if (result.games[i].home_team_id === winnerId) {
            return true;
          } else if (result.games[i].away_team_id === winnerId) {
            return true;
          }
        }
      }
    });
  }
}

export class TeamStats {
  pointsScored: number[];
  constructor(pointsScored: number[]) {
    this.pointsScored = pointsScored;
  }

  average(): number {
    return (
      this.pointsScored.reduce((a, b) => a + b, 0) / this.pointsScored.length
    );
  }

  variance(): number {
    let avg: number = this.average();
    return this.pointsScored.reduce((a, b) => a + (b - avg) ** 2, 0);
  }

  stdDev(): number {
    return Math.sqrt(this.variance());
  }
}

class SimulationResult {
  games: Game[];
  constructor(games: Game[]) {
    this.games = games;
  }

  simulate(): void {
    this.games.map((game) => {
      game.home_team_score = normalDistribution(24, 3);
      game.away_team_score = normalDistribution(24, 3);
      game.completed = true;
    });
  }
}

export class Schedule {
  games: Game[];
  constructor(games: Game[]) {
    this.games = games;
  }
}

export class Game {
  home_team_id: number;
  away_team_id: number;
  home_team_score: number;
  away_team_score: number;
  completed: boolean;
  week: number;

  constructor(
    home_team_id: number,
    away_team_id: number,
    home_team_score: number,
    away_team_score: number,
    completed: boolean,
    week: number
  ) {
    this.home_team_id = home_team_id;
    this.away_team_id = away_team_id;
    this.home_team_score = home_team_score;
    this.away_team_score = away_team_score;
    this.completed = completed;
    this.week = week;
  }
}

export class Filter {
  week: number;
  winnerId: number;
  constructor(week: number, winnerId: number) {
    this.week = week;
    this.winnerId = winnerId;
  }
}

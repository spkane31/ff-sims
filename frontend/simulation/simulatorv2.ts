/**
 * Generates a random number from a normal distribution.
 *
 * @param {number} mean - The mean value of the distribution.
 * @param {number} std - The standard deviation of the distribution.
 * @returns {number} - A random number from the normal distribution.
 */
export const normalDistribution = (mean: number, std: number): number => {
  let u = 0,
    v = 0;
  while (u === 0) u = Math.random();
  while (v === 0) v = Math.random();
  return (
    mean + std * Math.sqrt(-2.0 * Math.log(u)) * Math.cos(2.0 * Math.PI * v)
  );
};

/**
 * Shuffles an array in place.
 *
 * @param {any[]} array - The array to be shuffled.
 */
export const shuffle = (array: any[]): void => {
  let currentIndex = array.length;
  // While there remain elements to shuffle...
  while (currentIndex != 0) {
    // Pick a remaining element...
    let randomIndex = Math.floor(Math.random() * currentIndex);
    currentIndex--;
    // And swap it with the current element.
    [array[currentIndex], array[randomIndex]] = [
      array[randomIndex],
      array[currentIndex],
    ];
  }
};

export class SimulatorV2 {
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
    this.filteredResults = this.simulationResults;
  }

  addFilter(week: number, winnerId: number): void {
    for (let i = 0; i < this.filters.length; i++) {
      if (
        this.filters[i].week === week &&
        this.filters[i].winnerId === winnerId
      ) {
        return;
      }
    }
    this.filters.push(new Filter(week, winnerId));
    this.filter();
  }

  removeFilter(week: number, winnerId: number): void {
    this.filters = this.filters.filter(
      (filter) => filter.week !== week || filter.winnerId !== winnerId
    );
    this.filter();
  }

  clearFilters(): void {
    this.filters = [];
    this.filteredResults = this.simulationResults;
  }

  filter(): void {
    // let filteredResults: SimulationResult[] = this.simulationResults;
    let endResults: SimulationResult[] = [];

    for (let sr = 0; sr < this.simulationResults.length; sr++) {
      let add: boolean = true;
      for (let f = 0; f < this.filters.length; f++) {
        if (!filterMatches(this.filters[f], this.simulationResults[sr])) {
          add = false;
          break;
        }
      }
      if (add) {
        endResults.push(this.simulationResults[sr]);
      }
    }
    this.filteredResults = endResults;

    // for (let i = 0; i < this.filters.length; i++) {
    //   for (let j = 0; j < filteredResults.length; j++) {
    //     if (filterMatches(this.filters[i], filteredResults[j])) {
    //       filteredResults.splice(j, 1);
    //       j--;
    //     }
    //   }
    // }
    // this.filteredResults = filteredResults;
  }

  // this.filters.forEach((filter) => {
  //   this.filteredResults = this.filterResults(filter.week, filter.winnerId);
  // });
  // this.filters.map((filter) => {
  //   return (this.filteredResults = this.filterResults(
  //     filter.week,
  //     filter.winnerId
  //   ));
  // });
  // }

  // filterResults(week: number, winnerId: number): SimulationResult[] {
  //   return this.filteredResults.filter((result) => {
  //     for (let i: number = 0; i < result.games.length; i++) {
  //       if (result.games[i].week === week) {
  //         if (
  //           result.games[i].home_team_id === winnerId &&
  //           result.games[i].home_team_score > result.games[i].away_team_score
  //         ) {
  //           return true;
  //         } else if (
  //           result.games[i].away_team_id === winnerId &&
  //           result.games[i].away_team_score > result.games[i].home_team_score
  //         ) {
  //           return true;
  //         }
  //       }
  //     }
  //   });
  // }
}

export interface SimulatorV2 {
  schedule: Schedule;
  numSimulations: number;
  simulationResults: SimulationResult[];
  filteredResults: SimulationResult[];
  filters: Filter[];
  teamStats: Map<number, TeamStats>;
  leagueStats: TeamStats;
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
    return (
      this.pointsScored.reduce((a, b) => a + (b - avg) ** 2, 0) /
      (this.pointsScored.length - 1)
    );
  }

  stdDev(): number {
    return Math.sqrt(this.variance());
  }

  generateScore(): number {
    return normalDistribution(this.average(), this.stdDev());
  }
}

export interface TeamStats {
  pointsScored: number[];
}

export class SimulationResult {
  games: Game[];
  teamStats: Map<number, TeamStats>;
  constructor(games: Game[]) {
    this.games = [];
    games.forEach((game) => {
      this.games.push(
        new Game(
          game.home_team_id,
          game.away_team_id,
          game.home_team_score,
          game.away_team_score,
          game.completed,
          game.week
        )
      );
    });

    this.teamStats = new Map<number, TeamStats>();
    this.generateTeamStats();
  }

  generateTeamStats(): void {
    this.games.map((game) => {
      if (!this.teamStats.has(game.home_team_id)) {
        this.teamStats.set(game.home_team_id, new TeamStats([]));
      }
      if (!this.teamStats.has(game.away_team_id)) {
        this.teamStats.set(game.away_team_id, new TeamStats([]));
      }
      if (!game.completed) {
        return;
      }

      this.teamStats
        .get(game.home_team_id)
        ?.pointsScored.push(game.home_team_score);
      this.teamStats
        .get(game.away_team_id)
        ?.pointsScored.push(game.away_team_score);
    });
  }

  simulate(): void {
    this.games.map((game) => {
      if (game.completed) {
        return;
      }
      game.home_team_score = this.teamStats
        .get(game.home_team_id)
        ?.generateScore() as number;
      game.away_team_score = this.teamStats
        .get(game.away_team_id)
        ?.generateScore() as number;
      game.completed = true;
    });
  }
}

export interface SimulationResult {
  games: Game[];
}

export class Schedule {
  games: Game[];
  constructor(games: Game[]) {
    this.games = games;
  }
}

export interface Schedule {
  games: Game[];
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

  homeWins(): boolean {
    return this.home_team_score > this.away_team_score;
  }

  isTie(): boolean {
    return this.home_team_score === this.away_team_score;
  }

  awayWins(): boolean {
    return this.away_team_score > this.home_team_score;
  }
}

export interface Game {
  home_team_id: number;
  away_team_id: number;
  home_team_score: number;
  away_team_score: number;
  completed: boolean;
  week: number;
}

export class Filter {
  week: number;
  winnerId: number;
  constructor(week: number, winnerId: number) {
    this.week = week;
    this.winnerId = winnerId;
  }
}
export interface Filter {
  week: number;
  winnerId: number;
}

export const filterMatches = (
  filter: Filter,
  result: SimulationResult
): boolean => {
  for (let i = 0; i < result.games.length; i++) {
    if (result.games[i].week === filter.week) {
      if (result.games[i].isTie()) {
        return false;
      }
      if (
        result.games[i].home_team_id === filter.winnerId &&
        result.games[i].homeWins()
      ) {
        return true;
      } else if (
        result.games[i].away_team_id === filter.winnerId &&
        result.games[i].awayWins()
      ) {
        return true;
      }
    }
  }
  return false;
};

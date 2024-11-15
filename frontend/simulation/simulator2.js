/**
 * Generates a random number from a normal distribution.
 *
 * @param {number} mean - The mean value of the distribution.
 * @param {number} std - The standard deviation of the distribution.
 * @returns {number} - A random number from the normal distribution.
 */
export const normalDistribution = (mean, std) => {
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
export const shuffle = (array) => {
  let currentIndex = array.length;
  while (currentIndex != 0) {
    let randomIndex = Math.floor(Math.random() * currentIndex);
    currentIndex--;
    [array[currentIndex], array[randomIndex]] = [
      array[randomIndex],
      array[currentIndex],
    ];
  }
};

export default class SimulatorV2 {
  constructor(schedule) {
    this.schedule = schedule;
    this.numSimulations = 10000;
    this.simulationResults = [];
    this.filteredResults = [];
    this.teamStats = new Map();
    this.leagueStats = new TeamStats([]);
    this.filters = [];
    this.completed = false;
  }

  simulate(steps) {
    if (this.completed) {
      return;
    }
    let n = steps === undefined ? this.numSimulations : steps;
    for (let i = 0; i < n; i++) {
      let results = new SimulationResult(this.schedule.games);
      results.simulate();
      this.simulationResults.push(results);
    }
    this.filteredResults = this.simulationResults;
    this.completed = true;
  }

  // Returns a list of objects with fields {teamId, lastPlaceOdds, playoffOdds}
  getTeamData() {
    let teamData = new Map();

    this.schedule.games.forEach((week) => {
      week.forEach((game) => {
        if (!teamData.has(game.home_team_espn_id)) {
          teamData.set(game.home_team_espn_id, {
            teamId: game.home_team_espn_id,
            lastPlaceOdds: 0,
            playoffOdds: 0,
          });
        }
        if (!teamData.has(game.away_team_espn_id)) {
          teamData.set(game.away_team_espn_id, {
            teamId: game.away_team_espn_id,
            lastPlaceOdds: 0,
            playoffOdds: 0,
          });
        }
      });
    });

    teamData.forEach((_val, id) => {
      const lastPlace = this.lastPlaceOdds(id);
      const playoffs = this.playoffOdds(id);
      teamData.set(id, {
        teamId: id,
        lastPlaceOdds: lastPlace,
        playoffOdds: playoffs,
      });
    });

    return Array.from(teamData.values());
  }

  addFilter(week, winnerId) {
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

  removeFilter(week, winnerId) {
    this.filters = this.filters.filter(
      (filter) => filter.week !== week || filter.winnerId !== winnerId
    );
    this.filter();
  }

  removeAllFilters() {
    this.filters = [];
    this.filter();
  }

  clearFilters() {
    this.filters = [];
    this.filteredResults = this.simulationResults;
  }

  filter() {
    let endResults = [];
    for (let sr = 0; sr < this.simulationResults.length; sr++) {
      let add = true;
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
  }

  playoffOdds(teamId) {
    let count = 0;
    for (let i = 0; i < this.filteredResults.length; i++) {
      if (this.filteredResults[i].makesPlayoffs(teamId)) {
        count++;
      }
    }
    return this.numSimulations > 0 ? count / this.filteredResults.length : 0.0;
  }

  lastPlaceOdds(teamId) {
    let count = 0;
    for (let i = 0; i < this.filteredResults.length; i++) {
      if (this.filteredResults[i].getLastPlaceOdds(teamId)) {
        count++;
      }
    }
    return this.numSimulations > 0 ? count / this.filteredResults.length : 0.0;
  }
}

export class TeamStats {
  constructor(pointsScored) {
    this.pointsScored = pointsScored;
  }

  average() {
    return (
      this.pointsScored.reduce((a, b) => a + b, 0) / this.pointsScored.length
    );
  }

  variance() {
    let avg = this.average();
    return (
      this.pointsScored.reduce((a, b) => a + (b - avg) ** 2, 0) /
      (this.pointsScored.length - 1)
    );
  }

  stdDev() {
    return Math.sqrt(this.variance());
  }

  generateScore() {
    return normalDistribution(this.average(), this.stdDev());
  }
}

export class SimulationResult {
  constructor(weeks) {
    this.games = [];
    weeks.forEach((games) => {
      games.forEach((game) => {
        this.games.push(
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

    this.teamStats = new Map();
    this.generateTeamStats();
  }

  generateTeamStats() {
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

  simulate() {
    this.games.map((game) => {
      if (game.completed) {
        return;
      }
      game.home_team_score = this.teamStats
        .get(game.home_team_id)
        ?.generateScore();
      game.away_team_score = this.teamStats
        .get(game.away_team_id)
        ?.generateScore();
      game.completed = true;
    });
  }

  calculateStandings() {
    const standings = new Map();
    this.games.forEach((game) => {
      if (game.completed) {
        if (!standings.has(game.home_team_id)) {
          standings.set(
            game.home_team_id,
            new FinalStanding(game.home_team_id, 0, 0)
          );
        }
        if (!standings.has(game.away_team_id)) {
          standings.set(
            game.away_team_id,
            new FinalStanding(game.away_team_id, 0, 0)
          );
        }

        const homeTeam = standings.get(game.home_team_id);
        const awayTeam = standings.get(game.away_team_id);

        if (game.homeWins()) {
          if (homeTeam !== undefined) {
            homeTeam.wins++;
          }
        } else if (game.awayWins()) {
          if (awayTeam !== undefined) {
            awayTeam.wins++;
          }
        }
        if (homeTeam !== undefined) {
          homeTeam.pointsScored += game.home_team_score;
        }
        if (awayTeam !== undefined) {
          awayTeam.pointsScored += game.away_team_score;
        }
      }
    });
    return standings;
  }

  makesPlayoffs(teamId) {
    const standings = this.calculateStandings();

    const finalStandings = [];
    standings.forEach((value) => {
      finalStandings.push(value);
    });

    finalStandings.sort((a, b) => {
      if (a.wins !== b.wins) {
        return b.wins - a.wins;
      }
      return b.pointsScored - a.pointsScored;
    });

    for (let i = 0; i < 6; i++) {
      if (finalStandings[i].teamId === teamId) {
        return true;
      }
    }

    return false;
  }

  getLastPlaceOdds(teamId) {
    const standings = this.calculateStandings();

    const finalStandings = [];
    standings.forEach((value) => {
      finalStandings.push(value);
    });

    finalStandings.sort((a, b) => {
      if (a.wins !== b.wins) {
        return b.wins - a.wins;
      }
      return b.pointsScored - a.pointsScored;
    });

    return finalStandings[finalStandings.length - 1].teamId === teamId;
  }
}

class FinalStanding {
  constructor(teamId, wins, pointsScored) {
    this.teamId = teamId;
    this.wins = wins;
    this.pointsScored = pointsScored;
  }
}

export class Schedule {
  constructor(games) {
    this.games = games;
  }
}

export class Game {
  constructor(
    home_team_id,
    away_team_id,
    home_team_score,
    away_team_score,
    completed,
    week
  ) {
    this.home_team_id = home_team_id;
    this.away_team_id = away_team_id;
    this.home_team_score = home_team_score;
    this.away_team_score = away_team_score;
    this.completed = completed;
    this.week = week;
  }

  homeWins() {
    return this.home_team_score > this.away_team_score;
  }

  isTie() {
    return this.home_team_score === this.away_team_score;
  }

  awayWins() {
    return this.away_team_score > this.home_team_score;
  }
}

export class Filter {
  constructor(week, winnerId) {
    this.week = week;
    this.winnerId = winnerId;
  }
}

export const filterMatches = (filter, result) => {
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

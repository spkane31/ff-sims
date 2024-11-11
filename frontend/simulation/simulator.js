import { normalDistribution } from "../utils/math";

class Simulator {
  // simulator needs to take params of schedule and team averages
  // on the client side there needs to be an api for the data
  constructor(teamAvgs, schedule) {
    // schedule is a list of 14 weeks, each week is a list of 5 matchups
    this.schedule = schedule;

    // jitter is determined by weeks 1 - completed / weeks
    this.weeks = schedule.length;
    this.weeksCompleted = schedule.filter((week) =>
      week.every((matchup) => matchup.completed)
    ).length;

    // number of simulations run
    this.simulations = 0;

    // map of espn_id (int) -> Results()
    this.results = new Map();

    // map of espn_id (int) -> {average: float, std_dev: float}
    this.teamStats = new Map();
    Object.entries(teamAvgs).forEach(([_key, value]) => {
      if (value.id === -1) {
        this.leagueStats = {
          average: value.averageScore,
          std_dev: value.stddevScore,
        };
      } else {
        this.teamStats.set(value.id, {
          average: value.averageScore,
          std_dev: value.stddevScore,
        });
        this.results.set(value.id, new Results());
      }
    });

    this.idToOwner = new Map();
    Object.entries(teamAvgs).forEach(([_key, value]) => {
      this.idToOwner.set(value.id, value.owner);
    });

    this.epsilon = 0;
    this.previousStepFinalStandings = null;
  }

  // list of all teams, and projected wins, losses, points for, points against, playoff odds, last place odds
  getTeamScoringData() {
    const data = [];
    const sims = this.simulations;
    this.results.forEach((value, key) => {
      data.push({
        id: key,
        teamName: this.idToOwner.get(key),
        average: this.teamStats.get(key).average,
        std_dev: this.teamStats.get(key).std_dev,
        wins: sims === 0 ? 0.0 : value.wins / sims,
        losses: sims === 0 ? 0.0 : value.losses / sims,
        pointsFor: sims === 0 ? 0.0 : value.pointsFor / sims,
        pointsAgainst: sims === 0 ? 0.0 : value.pointsAgainst / sims,
        playoff_odds: sims === 0 ? 0.0 : value.madePlayoffs / sims,
        last_place_odds: sims === 0 ? 0.0 : value.lastPlace / sims,
        regular_season_result:
          sims === 0
            ? new Array(10).fill(0)
            : value.regularSeasonResult.map((num) => num / sims),
        playoff_result:
          sims === 0
            ? new Array(10).fill(0)
            : value.playoffResult.map((num) => num / sims),
      });
    });
    return data.sort((a, b) => {
      if (a.playoff_odds !== b.playoff_odds) {
        return b.playoff_odds - a.playoff_odds;
      } else if (a.last_place_odds !== b.last_place_odds) {
        return b.last_place_odds - a.last_place_odds;
      } else if (a.wins !== b.wins) {
        return b.wins - a.wins;
      }
      return b.average - a.average;
    });
  }

  totalGames() {
    return this.schedule.length * this.simulations;
  }

  getTeamIDs() {
    return Array.from(this.results.keys());
  }

  teamWin(teamID) {
    this.results.get(teamID).wins++;
  }

  teamLoss(teamID) {
    this.results.get(teamID).losses++;
  }

  teamPointsFor(teamID, points) {
    this.results.get(teamID).pointsFor += points;
  }

  teamPointsAgainst(teamID, points) {
    this.results.get(teamID).pointsAgainst += points;
  }

  getResults() {
    return this.results.entries();
  }

  getTeamResults(teamID) {
    return this.results.get(teamID);
  }

  getTeamStats(teamID) {
    return this.teamStats.get(teamID);
  }

  getPlayoffOdds({ teamId }) {
    const teamResults = this.results.get(teamId);
    if (this.simulations === 0) {
      return 0;
    }
    return teamResults.madePlayoffs / this.simulations;
  }

  getLastPlaceOdds({ teamId }) {
    const teamResults = this.results.get(teamId);
    if (this.simulations === 0) {
      return 0;
    }
    return teamResults.lastPlace / this.simulations;
  }

  step() {
    // create a map of team_id to SingleSeasonResults
    const singleSeasonResults = new SingleSeasonResults({
      teamAvgs: this.teamStats,
      leagueStats: this.leagueStats,
    });

    if (this.simulations > 0) {
      this.previousStepFinalStandings = this.getTeamScoringData();
    }

    this.schedule.forEach((game) => {
      // Code to print or process each game object in this.schedule
      game.forEach((matchup) => {
        if (!matchup.completed) {
          const { average: home_team_avg, std_dev: home_team_std_dev } =
            this.teamStats.get(matchup.home_team_espn_id);

          const { average: away_team_avg, std_dev: away_team_std_dev } =
            this.teamStats.get(matchup.away_team_espn_id);

          const { average: league_avg, std_dev: league_std_dev } =
            this.leagueStats;

          // random number between 0.05 and 0.25
          const league_jitter_home =
            Math.random() * (1 - this.weeksCompleted / this.weeks) + 0.05;
          const league_jitter_away =
            Math.random() * (1 - this.weeksCompleted / this.weeks) + 0.05;

          const home_score =
            (1 - league_jitter_home) *
              normalDistribution(home_team_avg, home_team_std_dev) +
            league_jitter_home * normalDistribution(league_avg, league_std_dev);

          const away_score =
            (1 - league_jitter_away) *
              normalDistribution(away_team_avg, away_team_std_dev) +
            league_jitter_away * normalDistribution(league_avg, league_std_dev);

          if (home_score > away_score) {
            singleSeasonResults.teamWin(matchup.home_team_espn_id);
            singleSeasonResults.teamLoss(matchup.away_team_espn_id);
          } else {
            singleSeasonResults.teamWin(matchup.away_team_espn_id);
            singleSeasonResults.teamLoss(matchup.home_team_espn_id);
          }
          singleSeasonResults.teamPointsFor(
            matchup.home_team_espn_id,
            home_score
          );
          singleSeasonResults.teamPointsAgainst(
            matchup.home_team_espn_id,
            away_score
          );
          singleSeasonResults.teamPointsFor(
            matchup.away_team_espn_id,
            away_score
          );
          singleSeasonResults.teamPointsAgainst(
            matchup.away_team_espn_id,
            home_score
          );
        } else {
          // In the case where the game is completed, just load in the stats
          const home_score = parseFloat(matchup.home_team_final_score);
          const away_score = parseFloat(matchup.away_team_final_score);
          if (home_score > away_score) {
            singleSeasonResults.teamWin(matchup.home_team_espn_id);
            singleSeasonResults.teamLoss(matchup.away_team_espn_id);
          } else {
            singleSeasonResults.teamWin(matchup.away_team_espn_id);
            singleSeasonResults.teamLoss(matchup.home_team_espn_id);
          }
          singleSeasonResults.teamPointsFor(
            matchup.home_team_espn_id,
            home_score
          );
          singleSeasonResults.teamPointsAgainst(
            matchup.home_team_espn_id,
            away_score
          );
          singleSeasonResults.teamPointsFor(
            matchup.away_team_espn_id,
            away_score
          );
          singleSeasonResults.teamPointsAgainst(
            matchup.away_team_espn_id,
            home_score
          );
        }
      });
    });

    singleSeasonResults.setFinalRankings();

    // Update the results map with the singleSeasonResults
    singleSeasonResults.results.forEach((value, key) => {
      const currentResults = this.results.get(key);
      currentResults.addSingleSeasonResults(value);
    });

    this.simulations++;
    this.updateEpsilon();
  }

  // updateEpsilon looks at the difference between the current simulations playoff odds per team
  // and the previous simulations playoff odds per team and does a sum of the value squared
  updateEpsilon() {
    if (this.simulations === 1) {
      this.epsilon = 0;
      return;
    }

    const currentStandings = this.getTeamScoringData();
    let sum = 0;
    for (let i = 0; i < currentStandings.length; i++) {
      const currentTeam = currentStandings[i];
      const previousTeam = this.previousStepFinalStandings[i];
      sum += Math.pow(currentTeam.wins - previousTeam.wins, 2);
    }
    this.epsilon = Math.sqrt(sum);
  }
}

class SingleSeasonResults {
  constructor({ teamAvgs, leagueStats }) {
    this.results = new Map();
    for (let [key, _teamStats] of teamAvgs.entries()) {
      this.results.set(key, new SingleTeamResults(key));
    }
    this.teamStats = teamAvgs;
    this.leagueStats = leagueStats;
  }

  teamWin(teamID) {
    this.results.get(teamID).wins++;
  }

  teamLoss(teamID) {
    this.results.get(teamID).losses++;
  }

  teamPointsFor(teamID, points) {
    this.results.get(teamID).pointsFor += points;
  }

  teamPointsAgainst(teamID, points) {
    this.results.get(teamID).pointsAgainst += points;
  }

  // sorts all the final results and marks madePlayoffs to true for teams in the top 6
  // and lastPlace as true for the last place team
  setFinalRankings() {
    // 1. First convert the results to a slice
    const resultsArray = Array.from(this.results.entries());

    // 2. Sort the results by wins, then points for, then points against
    resultsArray.sort((a, b) => {
      const aResults = a[1];
      const bResults = b[1];

      if (aResults.wins !== bResults.wins) {
        return bResults.wins - aResults.wins;
      }

      if (aResults.pointsFor !== bResults.pointsFor) {
        return bResults.pointsFor - aResults.pointsFor;
      }

      return aResults.pointsAgainst - bResults.pointsAgainst;
    });

    // 3. Mark the top 6 teams as madePlayoffs
    resultsArray.slice(0, 6).forEach((team) => {
      team[1].madePlayoffs = true;
    });

    // 4. Mark the last place team as lastPlace
    resultsArray[9][1].lastPlace = true;

    // 5. Assign the regular season result to each team
    resultsArray.forEach((team, index) => {
      team[1].regularSeasonResult = index + 1;
    });

    // TODO seankane 2024.08.29: something is wrong here, the probability of someone winning does not equal 1

    // 6. Simulate the playoffs with top two teams getting a bye, rest is single elimination
    // a. simulate 3rd vs 6th place teams
    const thirdPlaceTeam = resultsArray[2][1];
    const sixthPlaceTeam = resultsArray[5][1];

    const thirdWinnerTeam = simulateGame(
      thirdPlaceTeam,
      sixthPlaceTeam,
      this.teamStats,
      this.leagueStats
    );
    if (thirdWinnerTeam.id === thirdPlaceTeam.id) {
      sixthPlaceTeam.playoffResult = 6;
    } else {
      thirdPlaceTeam.playoffResult = 6;
    }

    const fourthPlaceTeam = resultsArray[3][1];
    const fifthPlaceTeam = resultsArray[4][1];

    const fourthWinnerTeam = simulateGame(
      fourthPlaceTeam,
      fifthPlaceTeam,
      this.teamStats,
      this.leagueStats
    );
    if (fourthWinnerTeam.id === fourthPlaceTeam.id) {
      fifthPlaceTeam.playoffResult = 5;
    } else {
      fourthPlaceTeam.playoffResult = 5;
    }

    const firstPlaceTeam = resultsArray[0][1];
    const secondPlaceTeam = resultsArray[1][1];

    const firstWinnerTeam = simulateGame(
      firstPlaceTeam,
      fourthWinnerTeam,
      this.teamStats,
      this.leagueStats
    );
    const secondWinnerTeam = simulateGame(
      secondPlaceTeam,
      thirdWinnerTeam,
      this.teamStats,
      this.leagueStats
    );
    const thirdConsolationWinnerTeam =
      firstWinnerTeam.id === firstPlaceTeam.id
        ? fourthWinnerTeam
        : firstPlaceTeam;
    const fourthConsolationWinnerTeam =
      secondWinnerTeam.id === secondPlaceTeam.id
        ? thirdWinnerTeam
        : secondPlaceTeam;

    // Simulate championship game
    const championshipWinner = simulateGame(
      firstWinnerTeam,
      secondWinnerTeam,
      this.teamStats,
      this.leagueStats
    );
    if (championshipWinner.id === firstWinnerTeam.id) {
      firstWinnerTeam.playoffResult = 1;
      secondWinnerTeam.playoffResult = 2;
    } else {
      firstWinnerTeam.playoffResult = 2;
      secondWinnerTeam.playoffResult = 1;
    }

    // Simulate 3rd place game
    const thirdPlaceWinner = simulateGame(
      thirdConsolationWinnerTeam,
      fourthConsolationWinnerTeam,
      this.teamStats,
      this.leagueStats
    );
    if (thirdPlaceWinner.id === thirdConsolationWinnerTeam.id) {
      thirdConsolationWinnerTeam.playoffResult = 3;
      fourthConsolationWinnerTeam.playoffResult = 4;
    } else {
      thirdConsolationWinnerTeam.playoffResult = 4;
      fourthConsolationWinnerTeam.playoffResult = 3;
    }
  }
}

// return true if the first team wins, false if the second team wins
const simulateGame = (first, second, teamAvgs, leagueStats) => {
  // Get first team averages
  const { average: firstAverage, std_dev: firstStdDev } = teamAvgs.get(
    first.id
  );

  // Get second team averages
  const { average: secondAverage, std_dev: secondStdDev } = teamAvgs.get(
    second.id
  );

  // Get league averages
  const { average: leagueAverage, std_dev: leagueStdDev } = leagueStats;

  // Random jitter between 0.05 and 0.25
  const firstJitter = Math.random() * 0.2 + 0.05;
  const secondJitter = Math.random() * 0.2 + 0.05;

  // Jittered league averages
  const firstLeagueJitter = normalDistribution(leagueAverage, leagueStdDev);
  const secondLeagueJitter = normalDistribution(leagueAverage, leagueStdDev);

  // Calculate scores
  const firstScore =
    (1 - firstJitter) * normalDistribution(firstAverage, firstStdDev) +
    firstJitter * firstLeagueJitter;

  const secondScore =
    (1 - secondJitter) * normalDistribution(secondAverage, secondStdDev) +
    secondJitter * secondLeagueJitter;

  return firstScore > secondScore ? first : second;
};

class SingleTeamResults {
  constructor(id) {
    this.id = id;
    this.wins = 0;
    this.losses = 0;
    this.pointsFor = 0;
    this.pointsAgainst = 0;
    this.madePlayoffs = false;
    this.lastPlace = false;
    this.regularSeasonResult = 0;
    this.playoffResult = -1;
  }
}

class Results {
  constructor() {
    this.wins = 0;
    this.losses = 0;
    this.pointsFor = 0;
    this.pointsAgainst = 0;
    this.madePlayoffs = 0;
    this.lastPlace = 0;
    this.regularSeasonResult = new Array(10).fill(0);
    this.playoffResult = new Array(10).fill(0);
  }

  games() {
    return this.wins + this.losses;
  }

  addSingleSeasonResults(singleSeasonResults) {
    this.wins += singleSeasonResults.wins;
    this.losses += singleSeasonResults.losses;
    this.pointsFor += singleSeasonResults.pointsFor;
    this.pointsAgainst += singleSeasonResults.pointsAgainst;
    this.madePlayoffs += singleSeasonResults.madePlayoffs ? 1 : 0;
    this.lastPlace += singleSeasonResults.lastPlace ? 1 : 0;
    this.regularSeasonResult[singleSeasonResults.regularSeasonResult - 1]++;
    this.playoffResult[singleSeasonResults.playoffResult - 1]++;
  }
}

export default Simulator;

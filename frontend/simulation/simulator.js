import team_to_id from "../data/team_to_id.json";
import team_id_to_owner from "../data/team_id_to_owner.json";
import schedule from "../data/schedule.json";
import team_avgs from "../data/team_avgs.json";
import { normalDistribution } from "../utils/math";

class Simulator {
  // simulator needs to take params of schedule and team_avgs
  // on the client side there needs to be an api for the data
  constructor(teamAvgs) {
    // schedule is a list of 14 weeks, each week is a list of 5 matchups
    this.schedule = schedule;

    // number of simulations run
    this.simulations = 0;

    // map of espn_id (int) -> Results()
    this.results = new Map();
    Object.entries(team_to_id).forEach(([key, value]) => {
      this.results.set(value, new Results());
    });

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
      }
    });
    // leagueStats is a {average: float, std_dev: float} object
  }

  // list of all teams, and projected wins, losses, points for, points against, playoff odds, last place odds
  getTeamScoringData() {
    const data = [];
    const sims = this.simulations;
    this.results.forEach((value, key) => {
      data.push({
        id: key,
        teamName: team_id_to_owner[parseInt(key)],
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
    return data;
  }

  totalGames() {
    return this.schedule.length * this.simulations;
  }

  getTeams() {
    return Array.from(this.results.keys()).map((teamId) => {
      return Object.keys(team_to_id).find((key) => team_to_id[key] === teamId);
    });
  }

  teamWin(teamName) {
    this.results.get(team_to_id[teamName]).wins++;
  }

  teamLoss(teamName) {
    this.results.get(team_to_id[teamName]).losses++;
  }

  teamPointsFor(teamName, points) {
    this.results.get(team_to_id[teamName]).pointsFor += points;
  }

  teamPointsAgainst(teamName, points) {
    this.results.get(team_to_id[teamName]).pointsAgainst += points;
  }

  getResults() {
    return this.results.entries();
  }

  getTeamResults(teamName) {
    return this.results.get(team_to_id[teamName]);
  }

  getTeamStats(teamName) {
    return this.teamStats.get(team_to_id[teamName]);
  }

  step() {
    // create a map of team_id to SingleSeasonResults
    const singleSeasonResults = new SingleSeasonResults();

    this.schedule.forEach((game) => {
      // Code to print or process each game object in this.schedule
      game.forEach((matchup) => {
        const { average: home_team_avg, std_dev: home_team_std_dev } =
          this.teamStats.get(team_to_id[matchup.home_team_owner]);

        const { average: away_team_avg, std_dev: away_team_std_dev } =
          this.teamStats.get(team_to_id[matchup.away_team_owner]);

        const { average: league_avg, std_dev: league_std_dev } =
          this.leagueStats;

        // random number between 0.05 and 0.25
        const league_jitter_home = Math.random() * 0.2 + 0.05;
        const league_jitter_away = Math.random() * 0.2 + 0.05;

        const home_score =
          (1 - league_jitter_home) *
            normalDistribution(home_team_avg, home_team_std_dev) +
          league_jitter_home * normalDistribution(league_avg, league_std_dev);

        const away_score =
          (1 - league_jitter_away) *
            normalDistribution(away_team_avg, away_team_std_dev) +
          league_jitter_away * normalDistribution(league_avg, league_std_dev);

        if (home_score > away_score) {
          singleSeasonResults.teamWin(matchup.home_team_owner);
          singleSeasonResults.teamLoss(matchup.away_team_owner);
        } else {
          singleSeasonResults.teamWin(matchup.away_team_owner);
          singleSeasonResults.teamLoss(matchup.home_team_owner);
        }
        singleSeasonResults.teamPointsFor(matchup.home_team_owner, home_score);
        singleSeasonResults.teamPointsAgainst(
          matchup.home_team_owner,
          away_score
        );
        singleSeasonResults.teamPointsFor(matchup.away_team_owner, away_score);
        singleSeasonResults.teamPointsAgainst(
          matchup.away_team_owner,
          home_score
        );
      });
    });

    singleSeasonResults.setFinalRankings();

    // Update the results map with the singleSeasonResults
    singleSeasonResults.results.forEach((value, key) => {
      const currentResults = this.results.get(key);
      currentResults.addSingleSeasonResults(value);
    });

    this.simulations++;
  }
}

class SingleSeasonResults {
  constructor() {
    this.results = new Map();
    Object.entries(team_to_id).forEach(([key, value]) => {
      this.results.set(value, new SingleTeamResults(value));
    });
  }

  teamWin(teamName) {
    this.results.get(team_to_id[teamName]).wins++;
  }

  teamLoss(teamName) {
    this.results.get(team_to_id[teamName]).losses++;
  }

  teamPointsFor(teamName, points) {
    this.results.get(team_to_id[teamName]).pointsFor += points;
  }

  teamPointsAgainst(teamName, points) {
    this.results.get(team_to_id[teamName]).pointsAgainst += points;
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

    const thirdWinnerTeam = simulateGame(thirdPlaceTeam, sixthPlaceTeam);
    if (thirdWinnerTeam.id === thirdPlaceTeam.id) {
      sixthPlaceTeam.playoffResult = 6;
    } else {
      thirdPlaceTeam.playoffResult = 6;
    }

    const fourthPlaceTeam = resultsArray[3][1];
    const fifthPlaceTeam = resultsArray[4][1];

    const fourthWinnerTeam = simulateGame(fourthPlaceTeam, fifthPlaceTeam);
    if (fourthWinnerTeam.id === fourthPlaceTeam.id) {
      fifthPlaceTeam.playoffResult = 5;
    } else {
      fourthPlaceTeam.playoffResult = 5;
    }

    const firstPlaceTeam = resultsArray[0][1];
    const secondPlaceTeam = resultsArray[1][1];

    const firstWinnerTeam = simulateGame(firstPlaceTeam, fourthWinnerTeam);
    const secondWinnerTeam = simulateGame(secondPlaceTeam, thirdWinnerTeam);
    const thirdConsolationWinnerTeam =
      firstWinnerTeam.id === firstPlaceTeam.id
        ? fourthWinnerTeam
        : firstPlaceTeam;
    const fourthConsolationWinnerTeam =
      secondWinnerTeam.id === secondPlaceTeam.id
        ? thirdWinnerTeam
        : secondPlaceTeam;

    // Simulate championship game
    const championshipWinner = simulateGame(firstWinnerTeam, secondWinnerTeam);
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
      fourthConsolationWinnerTeam
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
const simulateGame = (first, second) => {
  // Get first team averages
  const { average: firstAverage, std_dev: firstStdDev } =
    team_avgs[team_id_to_owner[first.id]];

  // Get second team averages
  const { average: secondAverage, std_dev: secondStdDev } =
    team_avgs[team_id_to_owner[second.id]];

  // Get league averages
  const { average: leagueAverage, std_dev: leagueStdDev } = team_avgs["League"];

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

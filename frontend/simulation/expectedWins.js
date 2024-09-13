import { normalDistribution, shuffle } from "../utils/math";

class ExpectedWins {
  // simulator needs to take params of schedule and team averages
  // on the client side there needs to be an api for the data
  constructor(teamAvgs, schedule) {
    // schedule is a list of 14 weeks, each week is a list of 5 matchups
    this.schedule = schedule;

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
    // leagueStats is a {average: float, std_dev: float} object

    this.idToOwner = new Map();
    Object.entries(teamAvgs).forEach(([_key, value]) => {
      this.idToOwner.set(value.id, value.owner);
    });
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

  step() {
    // create a map of team_id to SingleSeasonResults
    const singleSeasonResults = new SingleSeasonResults({
      teamAvgs: this.teamStats,
      leagueStats: this.leagueStats,
    });

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
  }

  // expectedWins finds the number of wins a team would expect to have scoring
  // the average points they scored in a game against a random schedule up to that point
  // in the season
  expectedWins(totalSims = 5000) {
    let totalWins = new Map();
    this.results.forEach((value, key) => {
      totalWins.set(key, 0);
    });

    for (let step = 0; step < totalSims; step++) {
      // iterate over the entire schedule
      this.schedule.forEach((week) => {
        // console.log(week);
        if (!week[0].completed) {
          // skip matchups that are not completed
          return;
        }

        // Get all performances in the week in a list
        let performances = [];
        week.forEach((matchup) => {
          performances.push({
            team_id: matchup.home_team_espn_id,
            score: parseFloat(matchup.home_team_final_score),
          });
          performances.push({
            team_id: matchup.away_team_espn_id,
            score: parseFloat(matchup.away_team_final_score),
          });
        });

        shuffle(performances);

        // compare 0 to 1, 2 to 3, etc.
        for (let i = 0; i < performances.length; i += 2) {
          const home_team_id = performances[i].team_id;
          const away_team_id = performances[i + 1].team_id;
          const home_score = performances[i].score;
          const away_score = performances[i + 1].score;

          if (home_score > away_score) {
            totalWins.set(home_team_id, totalWins.get(home_team_id) + 1);
          } else {
            totalWins.set(away_team_id, totalWins.get(away_team_id) + 1);
          }
        }
      });
    }
    let ret = [];
    totalWins.forEach((value, key) => {
      ret.push({ id: key, wins: value / totalSims });
    });
    return ret;
  }
}

export default ExpectedWins;

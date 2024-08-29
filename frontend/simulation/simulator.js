import team_to_id from "../data/team_to_id.json";
import team_id_to_owner from "../data/team_id_to_owner.json";
import schedule from "../data/schedule.json";
import team_avgs from "../data/team_avgs.json";

class Simulator {
  constructor() {
    this.schedule = schedule;
    this.simulations = 0;

    this.results = new Map();
    Object.entries(team_to_id).forEach(([key, value]) => {
      this.results.set(value, new Results());
    });

    this.teamStats = new Map();
    Object.entries(team_avgs).forEach(([key, value]) => {
      if (key === "League") {
        this.leagueStats = {
          average: value.average,
          std_dev: value.std_dev,
        };
        return;
      }
      this.teamStats.set(team_to_id[key], {
        average: value.average,
        std_dev: value.std_dev,
      });
    });
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
    console.log("single season results: ", singleSeasonResults);

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
        const league_jitter = Math.random() * 0.2 + 0.05;

        const home_score =
          (1 - league_jitter) *
            (Math.random() * home_team_std_dev + home_team_avg) +
          league_jitter * (Math.random() * league_std_dev + league_avg);
        const away_score =
          (1 - league_jitter) *
            (Math.random() * away_team_std_dev + away_team_avg) +
          league_jitter * (Math.random() * league_std_dev + league_avg);

        if (home_score > away_score) {
          this.teamWin(matchup.home_team_owner);
          this.teamLoss(matchup.away_team_owner);
        } else {
          this.teamWin(matchup.away_team_owner);
          this.teamLoss(matchup.home_team_owner);
        }
        this.teamPointsFor(matchup.home_team_owner, home_score);
        this.teamPointsAgainst(matchup.home_team_owner, away_score);
        this.teamPointsFor(matchup.away_team_owner, away_score);
        this.teamPointsAgainst(matchup.away_team_owner, home_score);
      });
    });

    this.simulations++;
    console.log(`Simulations: ${this.simulations}`);
  }
}

class SingleSeasonResults {
  constructor() {
    this.results = new Map();
    Object.entries(team_to_id).forEach(([key, value]) => {
      this.results.set(value, new SingleTeamResults());
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
}

class SingleTeamResults {
  constructor() {
    this.wins = 0;
    this.losses = 0;
    this.pointsFor = 0;
    this.pointsAgainst = 0;
    this.madePlayoffs = false;
    this.lastPlace = false;
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
  }
}

export default Simulator;

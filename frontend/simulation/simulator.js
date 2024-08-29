import team_to_id from "../data/team_to_id.json";
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
    this.schedule.forEach((game) => {
      // Code to print or process each game object in this.schedule
      game.forEach((matchup) => {
        console.log(matchup);
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
  }
}

class Results {
  constructor() {
    this.wins = 0;
    this.losses = 0;
    this.pointsFor = 0;
    this.pointsAgainst = 0;
  }

  games() {
    return this.wins + this.losses;
  }
}

export default Simulator;

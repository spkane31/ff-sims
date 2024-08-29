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
            (Math.random() * home_team_std_dev + home_team_avg) +
          league_jitter_home * (Math.random() * league_std_dev + league_avg);
        const away_score =
          (1 - league_jitter_away) *
            (Math.random() * away_team_std_dev + away_team_avg) +
          league_jitter_away * (Math.random() * league_std_dev + league_avg);

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

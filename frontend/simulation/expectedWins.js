import { normalDistribution, shuffle } from "../utils/math";

class ExpectedWins {
  // simulator needs to take params of schedule and team averages
  // on the client side there needs to be an api for the data
  constructor(schedule) {
    // schedule is a list of 14 weeks, each week is a list of 5 matchups
    this.schedule = schedule;

    // number of simulations run
    this.simulations = 0;

    // map of espn_id (int) -> Results()
    this.results = new Map();

    Object.entries(schedule[0]).forEach(([_key, value]) => {
      this.results.set(value.home_team_espn_id, 0);
      this.results.set(value.away_team_espn_id, 0);
    });
  }

  step() {
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
          this.results.set(home_team_id, this.results.get(home_team_id) + 1);
        } else {
          this.results.set(away_team_id, this.results.get(away_team_id) + 1);
        }
      }
    });
  }

  // expectedWins finds the number of wins a team would expect to have scoring
  // the average points they scored in a game against a random schedule up to that point
  // in the season
  expectedWins(totalSims = 10000) {
    for (let step = 0; step < totalSims; step++) {
      this.step();
    }
    let ret = [];
    this.results.forEach((value, key) => {
      ret.push({ id: key, wins: value / totalSims });
    });
    return ret;
  }
}

export default ExpectedWins;

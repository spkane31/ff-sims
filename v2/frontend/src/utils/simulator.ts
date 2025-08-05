import { normalDistribution } from "./math";
import {
  TeamStats,
  LeagueStats,
  Schedule,
  SingleTeamResult,
  TeamScoringData,
} from "../types/simulation";

class SingleSeasonResults {
  results: Map<number, SingleTeamResult>;
  teamStats: Map<number, TeamStats>;
  leagueStats: LeagueStats;

  constructor(teamAvgs: Map<number, TeamStats>, leagueStats: LeagueStats) {
    this.results = new Map();

    for (const [key] of teamAvgs.entries()) {
      this.results.set(key, {
        id: key,
        wins: 0,
        losses: 0,
        pointsFor: 0,
        pointsAgainst: 0,
        madePlayoffs: false,
        lastPlace: false,
        regularSeasonResult: 0,
        playoffResult: -1,
      });
    }
    this.teamStats = teamAvgs;
    this.leagueStats = leagueStats;
  }

  teamWin(teamID: number): void {
    const result = this.results.get(teamID);
    if (result) {
      result.wins++;
    }
  }

  teamLoss(teamID: number): void {
    const result = this.results.get(teamID);
    if (result) {
      result.losses++;
    }
  }

  teamPointsFor(teamID: number, points: number): void {
    const result = this.results.get(teamID);
    if (result) result.pointsFor += points;
  }

  teamPointsAgainst(teamID: number, points: number): void {
    const result = this.results.get(teamID);
    if (result) result.pointsAgainst += points;
  }

  setFinalRankings(): void {
    const resultsArray = Array.from(this.results.entries());

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

    // Mark top 6 teams as made playoffs
    resultsArray.slice(0, 6).forEach((team) => {
      team[1].madePlayoffs = true;
    });

    // Mark last place team
    if (resultsArray.length > 0) {
      resultsArray[resultsArray.length - 1][1].lastPlace = true;
    }

    // Assign regular season results
    resultsArray.forEach((team, index) => {
      team[1].regularSeasonResult = index + 1;
    });

    // Simulate playoffs
    this.simulatePlayoffs(resultsArray);
  }

  private simulatePlayoffs(resultsArray: [number, SingleTeamResult][]): void {
    if (resultsArray.length < 6) return;

    const teams = resultsArray.map(([, result]) => result);

    // Semifinal 1: 3rd vs 6th
    const semifinal1Winner = this.simulateGame(teams[2], teams[5]);
    const semifinal1Loser = semifinal1Winner === teams[2] ? teams[5] : teams[2];
    semifinal1Loser.playoffResult = 6;

    // Semifinal 2: 4th vs 5th
    const semifinal2Winner = this.simulateGame(teams[3], teams[4]);
    const semifinal2Loser = semifinal2Winner === teams[3] ? teams[4] : teams[3];
    semifinal2Loser.playoffResult = 5;

    // Championship semifinal 1: 1st vs semifinal2 winner
    const champSemi1Winner = this.simulateGame(teams[0], semifinal2Winner);
    const champSemi1Loser =
      champSemi1Winner === teams[0] ? semifinal2Winner : teams[0];

    // Championship semifinal 2: 2nd vs semifinal1 winner
    const champSemi2Winner = this.simulateGame(teams[1], semifinal1Winner);
    const champSemi2Loser =
      champSemi2Winner === teams[1] ? semifinal1Winner : teams[1];

    // Championship game
    const champion = this.simulateGame(champSemi1Winner, champSemi2Winner);
    const runnerUp =
      champion === champSemi1Winner ? champSemi2Winner : champSemi1Winner;

    champion.playoffResult = 1;
    runnerUp.playoffResult = 2;

    // Third place game
    const thirdPlace = this.simulateGame(champSemi1Loser, champSemi2Loser);
    const fourthPlace =
      thirdPlace === champSemi1Loser ? champSemi2Loser : champSemi1Loser;

    thirdPlace.playoffResult = 3;
    fourthPlace.playoffResult = 4;
  }

  private simulateGame(
    first: SingleTeamResult,
    second: SingleTeamResult
  ): SingleTeamResult {
    const firstStats = this.teamStats.get(first.id);
    const secondStats = this.teamStats.get(second.id);

    if (!firstStats || !secondStats) return first;

    const firstJitter = Math.random() * 0.2 + 0.05;
    const secondJitter = Math.random() * 0.2 + 0.05;

    const firstScore =
      (1 - firstJitter) *
        normalDistribution(firstStats.average, firstStats.std_dev) +
      firstJitter *
        normalDistribution(this.leagueStats.mean, this.leagueStats.stdDev);

    const secondScore =
      (1 - secondJitter) *
        normalDistribution(secondStats.average, secondStats.std_dev) +
      secondJitter *
        normalDistribution(this.leagueStats.mean, this.leagueStats.stdDev);

    return firstScore > secondScore ? first : second;
  }
}

class Results {
  wins: number = 0;
  losses: number = 0;
  pointsFor: number = 0;
  pointsAgainst: number = 0;
  madePlayoffs: number = 0;
  lastPlace: number = 0;
  regularSeasonResult: number[] = new Array(10).fill(0);
  playoffResult: number[] = new Array(10).fill(0);

  games(): number {
    return this.wins + this.losses;
  }

  addSingleSeasonResults(singleSeasonResults: SingleTeamResult): void {
    this.wins += singleSeasonResults.wins;
    this.losses += singleSeasonResults.losses;
    this.pointsFor += singleSeasonResults.pointsFor;
    this.pointsAgainst += singleSeasonResults.pointsAgainst;
    this.madePlayoffs += singleSeasonResults.madePlayoffs ? 1 : 0;
    this.lastPlace += singleSeasonResults.lastPlace ? 1 : 0;
    this.regularSeasonResult[singleSeasonResults.regularSeasonResult - 1]++;
    if (singleSeasonResults.playoffResult > 0) {
      this.playoffResult[singleSeasonResults.playoffResult - 1]++;
    }
  }
}

export class Simulator {
  schedule: Schedule;
  weeks: number;
  weeksCompleted: number;
  startWeek: number;
  simulations: number = 0;
  results: Map<number, Results>;
  teamStats: Map<number, TeamStats>;
  leagueStats: LeagueStats = { mean: 0, stdDev: 0 };
  idToOwner: Map<number, string>;
  epsilon: number = 0;
  previousStepFinalStandings: TeamScoringData[] | null = null;

  constructor(schedule: Schedule, startWeek: number = 1) {
    this.schedule = schedule;
    this.weeks = schedule.length;
    this.startWeek = startWeek;
    this.weeksCompleted = Math.min(
      startWeek - 1,
      schedule.filter((week) => week.every((matchup) => matchup.completed))
        .length
    );

    this.results = new Map();
    this.teamStats = new Map();
    this.idToOwner = new Map();

    // Calculate team and league stats from schedule
    this.calculateStatsFromSchedule();
  }

  private calculateStatsFromSchedule(): void {
    console.log(
      `Calculating stats from schedule. StartWeek: ${this.startWeek}`
    );

    // Map to store all scores for each team
    const teamScores = new Map<number, number[]>();
    const teamOwners = new Map<number, string>();

    // Process completed games up to startWeek
    this.schedule.forEach((week, weekIndex) => {
      const currentWeek = weekIndex + 1;
      console.log(
        `Processing week ${currentWeek}, startWeek is ${this.startWeek}`
      );

      // Fix: Only use data before startWeek for calculating averages
      if (currentWeek >= this.startWeek) {
        console.log(
          `Skipping week ${currentWeek} as it's >= startWeek ${this.startWeek}`
        );
        return;
      }

      week.forEach((matchup) => {
        if (!matchup.completed) {
          console.log(`Skipping incomplete matchup in week ${currentWeek}`);
          return;
        }

        const homeTeamId = matchup.homeTeamESPNID;
        const awayTeamId = matchup.awayTeamESPNID;
        const homeScore = matchup.homeTeamFinalScore;
        const awayScore = matchup.awayTeamFinalScore;

        console.log(
          `Processing matchup: Team ${homeTeamId} (type: ${typeof homeTeamId}) (${homeScore}) vs Team ${awayTeamId} (type: ${typeof awayTeamId}) (${awayScore})`
        );

        // Validate team IDs
        if (
          homeTeamId === undefined ||
          homeTeamId === null ||
          isNaN(homeTeamId)
        ) {
          console.error(`Invalid homeTeamId: ${homeTeamId}`);
          return;
        }
        if (
          awayTeamId === undefined ||
          awayTeamId === null ||
          isNaN(awayTeamId)
        ) {
          console.error(`Invalid awayTeamId: ${awayTeamId}`);
          return;
        }

        // Initialize arrays if they don't exist
        if (!teamScores.has(homeTeamId)) {
          teamScores.set(homeTeamId, []);
        }
        if (!teamScores.has(awayTeamId)) {
          teamScores.set(awayTeamId, []);
        }

        // Add scores
        teamScores.get(homeTeamId)!.push(homeScore);
        teamScores.get(awayTeamId)!.push(awayScore);

        // Store actual team owner names from the schedule data
        if (!teamOwners.has(homeTeamId)) {
          teamOwners.set(
            homeTeamId,
            matchup.homeTeamName || `Team ${homeTeamId}`
          );
        }
        if (!teamOwners.has(awayTeamId)) {
          teamOwners.set(
            awayTeamId,
            matchup.awayTeamName || `Team ${awayTeamId}`
          );
        }
      });
    });

    console.log(
      `Found ${teamScores.size} teams with game data:`,
      Array.from(teamScores.keys())
    );

    // If no completed games before startWeek, we need to process ALL teams from the schedule
    if (teamScores.size === 0) {
      console.log(
        "No completed games found before startWeek, processing all teams from schedule"
      );

      // Get all teams from the entire schedule
      this.schedule.forEach((week) => {
        week.forEach((matchup) => {
          const homeTeamId = matchup.homeTeamESPNID;
          const awayTeamId = matchup.awayTeamESPNID;

          if (!teamScores.has(homeTeamId)) {
            teamScores.set(homeTeamId, []);
            // Use actual team names from schedule data
            teamOwners.set(
              homeTeamId,
              matchup.homeTeamName || `Team ${homeTeamId}`
            );
          }
          if (!teamScores.has(awayTeamId)) {
            teamScores.set(awayTeamId, []);
            // Use actual team names from schedule data
            teamOwners.set(
              awayTeamId,
              matchup.awayTeamName || `Team ${awayTeamId}`
            );
          }

          // If this is a completed game, add the scores
          if (matchup.completed) {
            teamScores.get(homeTeamId)!.push(matchup.homeTeamFinalScore);
            teamScores.get(awayTeamId)!.push(matchup.awayTeamFinalScore);
          }
        });
      });
    }

    console.log(`Final team count: ${teamScores.size}`);

    // Calculate team stats
    teamScores.forEach((scores, teamId) => {
      if (scores.length === 0) {
        // If no completed games, use league average as fallback
        this.teamStats.set(teamId, { average: 100, std_dev: 15 });
        console.log(`Team ${teamId}: No games, using default stats (100, 15)`);
      } else {
        const average =
          scores.reduce((sum, score) => sum + score, 0) / scores.length;
        const variance =
          scores.reduce((sum, score) => sum + Math.pow(score - average, 2), 0) /
          scores.length;
        const std_dev = Math.sqrt(variance);

        this.teamStats.set(teamId, { average, std_dev });
        console.log(
          `Team ${teamId}: ${scores.length} games, avg: ${average.toFixed(
            2
          )}, std: ${std_dev.toFixed(2)}`
        );
      }

      this.results.set(teamId, new Results());
      this.idToOwner.set(teamId, teamOwners.get(teamId) || `Team ${teamId}`);
    });

    // Calculate league stats from all scores
    const allScores: number[] = [];
    teamScores.forEach((scores) => {
      allScores.push(...scores);
    });

    if (allScores.length > 0) {
      const leagueMean =
        allScores.reduce((sum, score) => sum + score, 0) / allScores.length;
      const leagueVariance =
        allScores.reduce(
          (sum, score) => sum + Math.pow(score - leagueMean, 2),
          0
        ) / allScores.length;
      const leagueStdDev = Math.sqrt(leagueVariance);

      this.leagueStats = { mean: leagueMean, stdDev: leagueStdDev };
      console.log(
        `League stats: mean ${leagueMean.toFixed(
          2
        )}, std ${leagueStdDev.toFixed(2)}`
      );
    } else {
      // Default league stats if no data
      this.leagueStats = { mean: 100, stdDev: 15 };
      console.log("Using default league stats (100, 15)");
    }

    console.log(
      `Final teamStats size: ${this.teamStats.size}, results size: ${this.results.size}`
    );
  }

  getTeamScoringData(): TeamScoringData[] {
    const data: TeamScoringData[] = [];
    const sims = this.simulations;

    this.results.forEach((value, key) => {
      const teamStats = this.teamStats.get(key);
      const teamName = this.idToOwner.get(key);

      if (teamStats && teamName) {
        data.push({
          id: key,
          teamName,
          average: teamStats.average,
          stdDev: teamStats.std_dev,
          wins: sims === 0 ? 0.0 : value.wins / sims,
          losses: sims === 0 ? 0.0 : value.losses / sims,
          pointsFor: sims === 0 ? 0.0 : value.pointsFor / sims,
          pointsAgainst: sims === 0 ? 0.0 : value.pointsAgainst / sims,
          playoffOdds: sims === 0 ? 0.0 : value.madePlayoffs / sims,
          lastPlaceOdds: sims === 0 ? 0.0 : value.lastPlace / sims,
          regularSeasonResult:
            sims === 0
              ? new Array(10).fill(0)
              : value.regularSeasonResult.map((num) => num / sims),
          playoffResult:
            sims === 0
              ? new Array(10).fill(0)
              : value.playoffResult.map((num) => num / sims),
        });
      }
    });

    return data.sort((a, b) => {
      if (a.playoffOdds !== b.playoffOdds) {
        return b.playoffOdds - a.playoffOdds;
      } else if (a.lastPlaceOdds !== b.lastPlaceOdds) {
        return b.lastPlaceOdds - a.lastPlaceOdds;
      } else if (a.wins !== b.wins) {
        return b.wins - a.wins;
      }
      return b.average - a.average;
    });
  }

  step(): void {
    const singleSeasonResults = new SingleSeasonResults(
      this.teamStats,
      this.leagueStats
    );

    if (this.simulations > 0) {
      this.previousStepFinalStandings = this.getTeamScoringData();
    }

    this.schedule.forEach((week, weekIndex) => {
      week.forEach((matchup) => {
        if (matchup.gameType !== "NONE") {
          console.log(
            `Skipping matchup in week ${weekIndex + 1} due to game type: ${
              matchup.gameType
            }`
          );
          return;
        }
        const currentWeek = weekIndex + 1;

        if (currentWeek < this.startWeek) {
          // For weeks before startWeek, use actual results if completed
          if (matchup.completed) {
            const homeScore = parseFloat(matchup.homeTeamFinalScore.toString());
            const awayScore = parseFloat(matchup.awayTeamFinalScore.toString());

            if (homeScore > awayScore) {
              singleSeasonResults.teamWin(matchup.homeTeamESPNID);
              singleSeasonResults.teamLoss(matchup.awayTeamESPNID);
            } else {
              singleSeasonResults.teamWin(matchup.awayTeamESPNID);
              singleSeasonResults.teamLoss(matchup.homeTeamESPNID);
            }

            singleSeasonResults.teamPointsFor(
              matchup.homeTeamESPNID,
              homeScore
            );
            singleSeasonResults.teamPointsAgainst(
              matchup.homeTeamESPNID,
              awayScore
            );
            singleSeasonResults.teamPointsFor(
              matchup.awayTeamESPNID,
              awayScore
            );
            singleSeasonResults.teamPointsAgainst(
              matchup.awayTeamESPNID,
              homeScore
            );
          }
        } else {
          // For weeks from startWeek onwards, always simulate (ignore actual results)
          const homeTeamStats = this.teamStats.get(matchup.homeTeamESPNID);
          const awayTeamStats = this.teamStats.get(matchup.awayTeamESPNID);

          if (!homeTeamStats || !awayTeamStats) return;

          const leagueJitterHome =
            Math.random() * (1 - this.weeksCompleted / this.weeks) + 0.05;
          const leagueJitterAway =
            Math.random() * (1 - this.weeksCompleted / this.weeks) + 0.05;

          const homeScore =
            (1 - leagueJitterHome) *
              normalDistribution(homeTeamStats.average, homeTeamStats.std_dev) +
            leagueJitterHome *
              normalDistribution(
                this.leagueStats.mean,
                this.leagueStats.stdDev
              );

          const awayScore =
            (1 - leagueJitterAway) *
              normalDistribution(awayTeamStats.average, awayTeamStats.std_dev) +
            leagueJitterAway *
              normalDistribution(
                this.leagueStats.mean,
                this.leagueStats.stdDev
              );

          if (homeScore > awayScore) {
            singleSeasonResults.teamWin(matchup.homeTeamESPNID);
            singleSeasonResults.teamLoss(matchup.awayTeamESPNID);
          } else {
            singleSeasonResults.teamWin(matchup.awayTeamESPNID);
            singleSeasonResults.teamLoss(matchup.homeTeamESPNID);
          }

          singleSeasonResults.teamPointsFor(matchup.homeTeamESPNID, homeScore);
          singleSeasonResults.teamPointsAgainst(
            matchup.homeTeamESPNID,
            awayScore
          );
          singleSeasonResults.teamPointsFor(matchup.awayTeamESPNID, awayScore);
          singleSeasonResults.teamPointsAgainst(
            matchup.awayTeamESPNID,
            homeScore
          );
        }
      });
    });

    singleSeasonResults.setFinalRankings();

    console.log("Single season results:", singleSeasonResults.results);

    // Update results with single season results
    singleSeasonResults.results.forEach((value, key) => {
      const currentResults = this.results.get(key);
      if (currentResults) {
        currentResults.addSingleSeasonResults(value);
      }
    });

    this.simulations++;
    this.updateEpsilon();
  }

  private updateEpsilon(): void {
    if (this.simulations === 1) {
      this.epsilon = 0;
      return;
    }

    if (!this.previousStepFinalStandings) return;

    const currentStandings = this.getTeamScoringData();
    let sum = 0;

    for (let i = 0; i < currentStandings.length; i++) {
      const currentTeam = currentStandings[i];
      const previousTeam = this.previousStepFinalStandings[i];
      sum += Math.pow(currentTeam.wins - previousTeam.wins, 2);
    }

    this.epsilon = Math.sqrt(sum);
  }

  // Utility methods
  totalGames(): number {
    return this.schedule.length * this.simulations;
  }

  getTeamIDs(): number[] {
    return Array.from(this.results.keys());
  }

  getResults(): IterableIterator<[number, Results]> {
    return this.results.entries();
  }

  getTeamResults(teamID: number): Results | undefined {
    return this.results.get(teamID);
  }

  getTeamStats(teamID: number): TeamStats | undefined {
    return this.teamStats.get(teamID);
  }
}

export default Simulator;

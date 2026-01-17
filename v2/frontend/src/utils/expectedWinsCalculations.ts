/**
 * Expected Wins Calculation Utilities
 *
 * These utilities calculate expected wins metrics from matchup data.
 * Expected wins are now stored per-matchup and calculated on the frontend.
 */

import { Matchup } from '../types/models';

/**
 * Calculate cumulative expected wins for a team through a specific week
 * @param matchups - Array of matchups
 * @param teamId - Internal team ID
 * @param throughWeek - Optional week to calculate through (defaults to all weeks)
 * @returns Cumulative expected wins (sum of per-week probabilities)
 */
export function calculateCumulativeExpectedWins(
  matchups: Matchup[],
  teamId: number,
  throughWeek?: number
): number {
  return matchups
    .filter(m => m.completed && !m.isPlayoff)
    .filter(m => !throughWeek || m.week <= throughWeek)
    .filter(m => m.homeTeamInternalId === teamId || m.awayTeamInternalId === teamId)
    .reduce((sum, m) => {
      const isHome = m.homeTeamInternalId === teamId;
      const expectedWin = isHome ? (m.homeExpectedWin || 0) : (m.awayExpectedWin || 0);
      return sum + expectedWin;
    }, 0);
}

/**
 * Calculate actual wins for a team
 * @param matchups - Array of matchups
 * @param teamId - Internal team ID
 * @param throughWeek - Optional week to calculate through
 * @returns Number of actual wins
 */
export function calculateActualWins(
  matchups: Matchup[],
  teamId: number,
  throughWeek?: number
): number {
  return matchups
    .filter(m => m.completed && !m.isPlayoff)
    .filter(m => !throughWeek || m.week <= throughWeek)
    .filter(m => m.homeTeamInternalId === teamId || m.awayTeamInternalId === teamId)
    .filter(m => {
      const isHome = m.homeTeamInternalId === teamId;
      return isHome
        ? m.homeScore > m.awayScore
        : m.awayScore > m.homeScore;
    }).length;
}

/**
 * Calculate win luck (actual wins - expected wins)
 * Positive values indicate lucky, negative values indicate unlucky
 * @param matchups - Array of matchups
 * @param teamId - Internal team ID
 * @returns Win luck value
 */
export function calculateWinLuck(
  matchups: Matchup[],
  teamId: number
): number {
  const expectedWins = calculateCumulativeExpectedWins(matchups, teamId);
  const actualWins = calculateActualWins(matchups, teamId);
  return actualWins - expectedWins;
}

/**
 * Team record for strength of schedule calculation
 */
export interface TeamRecord {
  wins: number;
  games: number;
}

/**
 * Calculate strength of schedule (average opponent win rate)
 * @param matchups - Array of matchups for the team
 * @param teamId - Internal team ID
 * @param allTeamRecords - Map of team ID to their record
 * @returns Average opponent win rate (0-1)
 */
export function calculateStrengthOfSchedule(
  matchups: Matchup[],
  teamId: number,
  allTeamRecords: Map<number, TeamRecord>
): number {
  const teamMatchups = matchups.filter(m =>
    m.homeTeamInternalId === teamId || m.awayTeamInternalId === teamId
  );

  if (teamMatchups.length === 0) return 0;

  const opponentWinRates = teamMatchups.map(m => {
    const opponentId = m.homeTeamInternalId === teamId ? m.awayTeamInternalId : m.homeTeamInternalId;
    const record = allTeamRecords.get(opponentId);

    if (!record || record.games === 0) {
      return 0.5; // Default to 0.5 if no data
    }

    return record.wins / record.games;
  });

  const sum = opponentWinRates.reduce((acc, rate) => acc + rate, 0);
  return sum / opponentWinRates.length;
}

/**
 * Build team records map from matchups
 * @param matchups - Array of matchups
 * @returns Map of team ID to their record
 */
export function buildTeamRecords(matchups: Matchup[]): Map<number, TeamRecord> {
  const records = new Map<number, TeamRecord>();

  matchups
    .filter(m => m.completed && !m.isPlayoff)
    .forEach(m => {
      // Initialize records if needed
      if (!records.has(m.homeTeamInternalId)) {
        records.set(m.homeTeamInternalId, { wins: 0, games: 0 });
      }
      if (!records.has(m.awayTeamInternalId)) {
        records.set(m.awayTeamInternalId, { wins: 0, games: 0 });
      }

      const homeRecord = records.get(m.homeTeamInternalId)!;
      const awayRecord = records.get(m.awayTeamInternalId)!;

      // Update games count
      homeRecord.games++;
      awayRecord.games++;

      // Update wins
      if (m.homeScore > m.awayScore) {
        homeRecord.wins++;
      } else if (m.awayScore > m.homeScore) {
        awayRecord.wins++;
      }
      // Ties don't count as wins
    });

  return records;
}

/**
 * Weekly progression point for charts
 */
export interface WeeklyProgressionPoint {
  week: number;
  expectedWins: number;
  actualWins: number;
  weeklyExpectedWin: number;
  weeklyActualWin: boolean;
  pointDifferential?: number;
}

/**
 * Calculate weekly progression for charts
 * @param matchups - Array of matchups
 * @param teamId - Internal team ID
 * @returns Array of weekly progression points
 */
export function calculateWeeklyProgression(
  matchups: Matchup[],
  teamId: number
): WeeklyProgressionPoint[] {
  const teamMatchups = matchups
    .filter(m => m.completed && !m.isPlayoff)
    .filter(m => m.homeTeamInternalId === teamId || m.awayTeamInternalId === teamId)
    .sort((a, b) => a.week - b.week);

  const weeks = [...new Set(teamMatchups.map(m => m.week))].sort((a, b) => a - b);

  return weeks.map(week => {
    const expectedWins = calculateCumulativeExpectedWins(matchups, teamId, week);
    const actualWins = calculateActualWins(matchups, teamId, week);

    // Find this week's matchup
    const weekMatchup = teamMatchups.find(m => m.week === week);
    const isHome = weekMatchup?.homeTeamInternalId === teamId;
    const weeklyExpectedWin = weekMatchup
      ? (isHome ? (weekMatchup.homeExpectedWin || 0) : (weekMatchup.awayExpectedWin || 0))
      : 0;
    const weeklyActualWin = weekMatchup
      ? (isHome ? weekMatchup.homeScore > weekMatchup.awayScore : weekMatchup.awayScore > weekMatchup.homeScore)
      : false;
    const pointDifferential = weekMatchup
      ? (isHome ? weekMatchup.homeScore - weekMatchup.awayScore : weekMatchup.awayScore - weekMatchup.homeScore)
      : undefined;

    return {
      week,
      expectedWins,
      actualWins,
      weeklyExpectedWin,
      weeklyActualWin,
      pointDifferential,
    };
  });
}

/**
 * All-time expected wins summary
 */
export interface AllTimeExpectedWins {
  totalExpectedWins: number;
  totalExpectedLosses: number;
  totalActualWins: number;
  totalActualLosses: number;
  totalWinLuck: number;
  seasonsPlayed: number;
}

/**
 * Calculate all-time expected wins across multiple seasons
 * @param matchupsBySeason - Map of season year to matchups
 * @param teamId - Internal team ID
 * @returns All-time expected wins summary
 */
export function calculateAllTimeExpectedWins(
  matchupsBySeason: Map<number, Matchup[]>,
  teamId: number
): AllTimeExpectedWins {
  let totalExpectedWins = 0;
  let totalActualWins = 0;
  let totalGames = 0;

  matchupsBySeason.forEach((matchups) => {
    totalExpectedWins += calculateCumulativeExpectedWins(matchups, teamId);
    totalActualWins += calculateActualWins(matchups, teamId);

    const gamesPlayed = matchups.filter(m =>
      m.completed && !m.isPlayoff &&
      (m.homeTeamInternalId === teamId || m.awayTeamInternalId === teamId)
    ).length;

    totalGames += gamesPlayed;
  });

  return {
    totalExpectedWins,
    totalExpectedLosses: totalGames - totalExpectedWins,
    totalActualWins,
    totalActualLosses: totalGames - totalActualWins,
    totalWinLuck: totalActualWins - totalExpectedWins,
    seasonsPlayed: matchupsBySeason.size,
  };
}

/**
 * Find luckiest week (highest positive luck)
 * @param matchups - Array of matchups
 * @param teamId - Internal team ID
 * @returns Week number and luck value, or null if no weeks
 */
export function findLuckiestWeek(
  matchups: Matchup[],
  teamId: number
): { week: number; luck: number; expectedWin: number; actualWin: boolean } | null {
  const progression = calculateWeeklyProgression(matchups, teamId);

  if (progression.length === 0) return null;

  let luckiestWeek = progression[0];
  let maxLuck = luckiestWeek.weeklyActualWin ? (1 - luckiestWeek.weeklyExpectedWin) : -luckiestWeek.weeklyExpectedWin;

  progression.forEach(point => {
    const luck = point.weeklyActualWin ? (1 - point.weeklyExpectedWin) : -point.weeklyExpectedWin;
    if (luck > maxLuck) {
      maxLuck = luck;
      luckiestWeek = point;
    }
  });

  return {
    week: luckiestWeek.week,
    luck: maxLuck,
    expectedWin: luckiestWeek.weeklyExpectedWin,
    actualWin: luckiestWeek.weeklyActualWin,
  };
}

/**
 * Find unluckiest week (highest negative luck)
 * @param matchups - Array of matchups
 * @param teamId - Internal team ID
 * @returns Week number and luck value, or null if no weeks
 */
export function findUnluckiestWeek(
  matchups: Matchup[],
  teamId: number
): { week: number; luck: number; expectedWin: number; actualWin: boolean } | null {
  const progression = calculateWeeklyProgression(matchups, teamId);

  if (progression.length === 0) return null;

  let unluckiestWeek = progression[0];
  let minLuck = unluckiestWeek.weeklyActualWin ? (1 - unluckiestWeek.weeklyExpectedWin) : -unluckiestWeek.weeklyExpectedWin;

  progression.forEach(point => {
    const luck = point.weeklyActualWin ? (1 - point.weeklyExpectedWin) : -point.weeklyExpectedWin;
    if (luck < minLuck) {
      minLuck = luck;
      unluckiestWeek = point;
    }
  });

  return {
    week: unluckiestWeek.week,
    luck: minLuck,
    expectedWin: unluckiestWeek.weeklyExpectedWin,
    actualWin: unluckiestWeek.weeklyActualWin,
  };
}

/**
 * Calculate league-wide statistics
 */
export interface LeagueExpectedWinsStats {
  averageExpectedWins: number;
  averageWinLuck: number;
  luckiestTeam: { teamId: number; luck: number } | null;
  unluckiestTeam: { teamId: number; luck: number } | null;
}

/**
 * Calculate league-wide expected wins statistics
 * @param matchups - Array of all matchups
 * @param teamIds - Array of team IDs to analyze
 * @returns League statistics
 */
export function calculateLeagueStats(
  matchups: Matchup[],
  teamIds: number[]
): LeagueExpectedWinsStats {
  if (teamIds.length === 0) {
    return {
      averageExpectedWins: 0,
      averageWinLuck: 0,
      luckiestTeam: null,
      unluckiestTeam: null,
    };
  }

  let totalExpectedWins = 0;
  let totalLuck = 0;
  let luckiestTeam: { teamId: number; luck: number } | null = null;
  let unluckiestTeam: { teamId: number; luck: number } | null = null;

  teamIds.forEach(teamId => {
    const expectedWins = calculateCumulativeExpectedWins(matchups, teamId);
    const luck = calculateWinLuck(matchups, teamId);

    totalExpectedWins += expectedWins;
    totalLuck += luck;

    if (luckiestTeam === null || luck > luckiestTeam.luck) {
      luckiestTeam = { teamId, luck };
    }

    if (unluckiestTeam === null || luck < unluckiestTeam.luck) {
      unluckiestTeam = { teamId, luck };
    }
  });

  return {
    averageExpectedWins: totalExpectedWins / teamIds.length,
    averageWinLuck: totalLuck / teamIds.length,
    luckiestTeam,
    unluckiestTeam,
  };
}

import React from 'react';
import { SeasonExpectedWins, WeeklyExpectedWins } from '../services/expectedWinsService';

interface ExpectedWinsBannerProps {
  seasonData: SeasonExpectedWins[];
  weeklyData?: WeeklyExpectedWins[];
  isLoading: boolean;
  currentYear: number;
}

interface ExpectedWinsStats {
  averageExpectedWins: number;
  averageWinLuck: number;
  luckiestTeam: { name: string; luck: number } | null;
  unluckiestTeam: { name: string; luck: number } | null;
  totalTeams: number;
}

function calculateExpectedWinsStats(seasonData: SeasonExpectedWins[], weeklyData?: WeeklyExpectedWins[]): ExpectedWinsStats {
  // If we have season data, use it directly
  if (seasonData && seasonData.length > 0) {
    const totalExpectedWins = seasonData.reduce((sum, team) => sum + team.expected_wins, 0);
    const totalWinLuck = seasonData.reduce((sum, team) => sum + team.win_luck, 0);

    let luckiestTeam = seasonData[0];
    let unluckiestTeam = seasonData[0];

    seasonData.forEach((team) => {
      if (team.win_luck > luckiestTeam.win_luck) {
        luckiestTeam = team;
      }
      if (team.win_luck < unluckiestTeam.win_luck) {
        unluckiestTeam = team;
      }
    });

    return {
      averageExpectedWins: totalExpectedWins / seasonData.length,
      averageWinLuck: totalWinLuck / seasonData.length,
      luckiestTeam: luckiestTeam.team ? { 
        name: luckiestTeam.team.owner_name, 
        luck: luckiestTeam.win_luck 
      } : null,
      unluckiestTeam: unluckiestTeam.team ? { 
        name: unluckiestTeam.team.owner_name, 
        luck: unluckiestTeam.win_luck 
      } : null,
      totalTeams: seasonData.length,
    };
  }

  // If no season data but we have weekly data, derive stats from latest week per team
  if (weeklyData && weeklyData.length > 0) {
    // Get the latest week data for each team
    const latestWeekPerTeam = new Map<number, WeeklyExpectedWins>();
    weeklyData.forEach((week) => {
      const existing = latestWeekPerTeam.get(week.team_id);
      if (!existing || week.week > existing.week) {
        latestWeekPerTeam.set(week.team_id, week);
      }
    });

    const latestWeekData = Array.from(latestWeekPerTeam.values());
    const totalExpectedWins = latestWeekData.reduce((sum, team) => sum + team.expected_wins, 0);
    const totalWinLuck = latestWeekData.reduce((sum, team) => sum + team.win_luck, 0);

    let luckiestTeam = latestWeekData[0];
    let unluckiestTeam = latestWeekData[0];

    latestWeekData.forEach((team) => {
      if (team.win_luck > luckiestTeam.win_luck) {
        luckiestTeam = team;
      }
      if (team.win_luck < unluckiestTeam.win_luck) {
        unluckiestTeam = team;
      }
    });

    return {
      averageExpectedWins: totalExpectedWins / latestWeekData.length,
      averageWinLuck: totalWinLuck / latestWeekData.length,
      luckiestTeam: luckiestTeam.team ? { 
        name: luckiestTeam.team.owner_name, 
        luck: luckiestTeam.win_luck 
      } : null,
      unluckiestTeam: unluckiestTeam.team ? { 
        name: unluckiestTeam.team.owner_name, 
        luck: unluckiestTeam.win_luck 
      } : null,
      totalTeams: latestWeekData.length,
    };
  }

  // No data available
  return {
    averageExpectedWins: 0,
    averageWinLuck: 0,
    luckiestTeam: null,
    unluckiestTeam: null,
    totalTeams: 0,
  };
}

export default function ExpectedWinsBanner({ seasonData, weeklyData, isLoading, currentYear }: ExpectedWinsBannerProps) {
  if (isLoading) {
    return (
      <div className="bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-900/20 dark:to-purple-900/20 p-6 rounded-lg shadow-md mb-8">
        <div className="flex items-center justify-center h-20">
          <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-2">Loading expected wins data...</span>
        </div>
      </div>
    );
  }

  const stats = calculateExpectedWinsStats(seasonData, weeklyData);

  if (stats.totalTeams === 0) {
    return (
      <div className="bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-900/20 dark:to-purple-900/20 p-6 rounded-lg shadow-md mb-8">
        <div className="text-center">
          <h2 className="text-xl font-bold text-gray-700 dark:text-gray-300 mb-2">
            Expected Wins Analysis - {currentYear}
          </h2>
          <p className="text-gray-600 dark:text-gray-400">
            No expected wins data available for this season.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-900/20 dark:to-purple-900/20 p-6 rounded-lg shadow-md mb-8">
      <div className="text-center mb-6">
        <h2 className="text-2xl font-bold text-gray-800 dark:text-gray-200 mb-2">
          Expected Wins Analysis - {currentYear}
        </h2>
        <p className="text-gray-600 dark:text-gray-400">
          How many wins each team &ldquo;should&rdquo; have based on their scoring performance
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <div className="text-center">
          <div className="text-3xl font-bold text-blue-600 dark:text-blue-400">
            {stats.averageExpectedWins.toFixed(1)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Average Expected Wins
          </div>
        </div>

        <div className="text-center">
          <div className={`text-3xl font-bold ${
            stats.averageWinLuck > 0 
              ? 'text-green-600 dark:text-green-400' 
              : stats.averageWinLuck < 0 
              ? 'text-red-600 dark:text-red-400' 
              : 'text-gray-600 dark:text-gray-400'
          }`}>
            {stats.averageWinLuck > 0 ? '+' : ''}{stats.averageWinLuck.toFixed(1)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Average Win Luck
          </div>
        </div>

        {stats.luckiestTeam && (
          <div className="text-center">
            <div className="text-2xl font-bold text-green-600 dark:text-green-400">
              +{stats.luckiestTeam.luck.toFixed(1)}
            </div>
            <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
              Luckiest Team
            </div>
            <div className="text-xs font-medium text-gray-700 dark:text-gray-300">
              {stats.luckiestTeam.name}
            </div>
          </div>
        )}

        {stats.unluckiestTeam && (
          <div className="text-center">
            <div className="text-2xl font-bold text-red-600 dark:text-red-400">
              {stats.unluckiestTeam.luck.toFixed(1)}
            </div>
            <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
              Unluckiest Team
            </div>
            <div className="text-xs font-medium text-gray-700 dark:text-gray-300">
              {stats.unluckiestTeam.name}
            </div>
          </div>
        )}
      </div>

      <div className="mt-6 text-center">
        <p className="text-xs text-gray-500 dark:text-gray-400">
          Win Luck = Actual Wins - Expected Wins. Positive values indicate teams that have been lucky with their schedule.
        </p>
      </div>
    </div>
  );
}
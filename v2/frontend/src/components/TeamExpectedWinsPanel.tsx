import React from "react";
import {
  WeeklyExpectedWins,
  SeasonExpectedWins,
} from "../services/expectedWinsService";

interface TeamExpectedWinsPanelProps {
  teamId: number;
  progressionData?: WeeklyExpectedWins[];
  seasonData: SeasonExpectedWins[];
  isLoading: boolean;
  currentYear: number;
}

interface TeamExpectedWinsStats {
  currentExpectedWins: number;
  currentActualWins: number;
  winLuck: number;
  strengthOfSchedule: number;
  luckiestWeek: { week: number; luck: number } | null;
  unluckiestWeek: { week: number; luck: number } | null;
}

function calculateTeamExpectedWinsStats(
  progressionData: WeeklyExpectedWins[] | undefined,
  seasonData: SeasonExpectedWins[]
): TeamExpectedWinsStats {
  // Use season data if available, otherwise fall back to progression data
  const seasonStats = seasonData.length > 0 ? seasonData[0] : null;

  if (seasonStats) {
    // Use season data when available
    return {
      currentExpectedWins: seasonStats.expected_wins,
      currentActualWins: seasonStats.actual_wins,
      winLuck: seasonStats.win_luck,
      strengthOfSchedule: seasonStats.strength_of_schedule,
      luckiestWeek: null, // No weekly breakdown from season data
      unluckiestWeek: null,
    };
  }

  if (!progressionData || progressionData.length === 0) {
    return {
      currentExpectedWins: 0,
      currentActualWins: 0,
      winLuck: 0,
      strengthOfSchedule: 0,
      luckiestWeek: null,
      unluckiestWeek: null,
    };
  }

  // Get the most recent week's data for current stats
  const latestWeek = progressionData[progressionData.length - 1];

  // Find luckiest and unluckiest weeks (highest and lowest weekly win probability that resulted in opposite outcome)
  let luckiestWeek: { week: number; luck: number } | null = null;
  let unluckiestWeek: { week: number; luck: number } | null = null;

  progressionData.forEach((weekData) => {
    // Calculate luck as the difference between actual result and expected probability
    const actualResult = weekData.weekly_actual_win ? 1 : 0;
    const weekLuck = actualResult - weekData.weekly_win_probability;

    if (!luckiestWeek || weekLuck > luckiestWeek.luck) {
      luckiestWeek = { week: weekData.week, luck: weekLuck };
    }

    if (!unluckiestWeek || weekLuck < unluckiestWeek.luck) {
      unluckiestWeek = { week: weekData.week, luck: weekLuck };
    }
  });

  return {
    currentExpectedWins: latestWeek.expected_wins,
    currentActualWins: latestWeek.actual_wins,
    winLuck: latestWeek.win_luck,
    strengthOfSchedule: latestWeek.strength_of_schedule,
    luckiestWeek,
    unluckiestWeek,
  };
}

export default function TeamExpectedWinsPanel({
  progressionData,
  seasonData,
  isLoading,
  currentYear,
}: TeamExpectedWinsPanelProps) {
  if (isLoading) {
    return (
      <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
        <div className="flex items-center justify-center h-32">
          <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-2">Loading expected wins data...</span>
        </div>
      </div>
    );
  }

  const stats = calculateTeamExpectedWinsStats(progressionData, seasonData);

  if (
    seasonData.length === 0 &&
    (!progressionData || progressionData.length === 0)
  ) {
    return (
      <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
        <h2 className="text-xl font-semibold mb-4">
          Expected Wins Analysis - {currentYear}
        </h2>
        <div className="text-center text-gray-500 dark:text-gray-400">
          No expected wins data available for this team in {currentYear}.
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
      <h2 className="text-xl font-semibold mb-6">
        Expected Wins Analysis - {currentYear}
      </h2>

      {/* Main Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-6">
        <div className="text-center">
          <div className="text-3xl font-bold text-blue-600 dark:text-blue-400">
            {stats.currentExpectedWins.toFixed(1)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Expected Wins
          </div>
        </div>

        <div className="text-center">
          <div className="text-3xl font-bold text-gray-800 dark:text-gray-200">
            {stats.currentActualWins}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Actual Wins
          </div>
        </div>

        <div className="text-center">
          <div
            className={`text-3xl font-bold ${
              stats.winLuck > 0
                ? "text-green-600 dark:text-green-400"
                : stats.winLuck < 0
                ? "text-red-600 dark:text-red-400"
                : "text-gray-600 dark:text-gray-400"
            }`}
          >
            {stats.winLuck > 0 ? "+" : ""}
            {stats.winLuck.toFixed(1)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Win Luck
          </div>
        </div>

        <div className="text-center">
          <div className="text-3xl font-bold text-purple-600 dark:text-purple-400">
            {stats.strengthOfSchedule.toFixed(2)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Strength of Schedule
          </div>
        </div>
      </div>

      {/* Additional Insights */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        {stats.luckiestWeek && (
          <div className="bg-green-50 dark:bg-green-900/20 p-4 rounded-lg">
            <h3 className="text-sm font-medium text-green-700 dark:text-green-300 mb-2">
              Luckiest Week
            </h3>
            <div className="text-lg font-bold text-green-600 dark:text-green-400">
              Week {stats.luckiestWeek.week}
            </div>
            <div className="text-xs text-green-600 dark:text-green-400">
              +{stats.luckiestWeek.luck.toFixed(2)} luck
            </div>
          </div>
        )}

        {stats.unluckiestWeek && (
          <div className="bg-red-50 dark:bg-red-900/20 p-4 rounded-lg">
            <h3 className="text-sm font-medium text-red-700 dark:text-red-300 mb-2">
              Unluckiest Week
            </h3>
            <div className="text-lg font-bold text-red-600 dark:text-red-400">
              Week {stats.unluckiestWeek.week}
            </div>
            <div className="text-xs text-red-600 dark:text-red-400">
              {stats.unluckiestWeek.luck.toFixed(2)} luck
            </div>
          </div>
        )}
      </div>

      {/* Performance Interpretation */}
      <div className="bg-blue-50 dark:bg-blue-900/20 p-4 rounded-lg">
        <h3 className="text-sm font-semibold text-blue-800 dark:text-blue-300 mb-2">
          Performance Analysis
        </h3>
        <div className="text-sm text-blue-700 dark:text-blue-300">
          {stats.winLuck > 1 ? (
            <>
              🍀 This team has been <strong>lucky</strong> with their schedule,
              winning {stats.winLuck.toFixed(1)} more games than expected.
            </>
          ) : stats.winLuck < -1 ? (
            <>
              😤 This team has been <strong>unlucky</strong>, winning{" "}
              {Math.abs(stats.winLuck).toFixed(1)} fewer games than expected.
            </>
          ) : (
            <>
              ⚖️ This team&apos;s record closely matches their expected
              performance based on scoring.
            </>
          )}

          {stats.strengthOfSchedule > 0.5 ? (
            <>
              {" "}
              They&apos;ve faced tougher opponents than average (SOS:{" "}
              {stats.strengthOfSchedule.toFixed(2)}).
            </>
          ) : stats.strengthOfSchedule < -0.5 ? (
            <>
              {" "}
              They&apos;ve had an easier schedule than average (SOS:{" "}
              {stats.strengthOfSchedule.toFixed(2)}).
            </>
          ) : (
            <> Their schedule strength has been about average.</>
          )}
        </div>
      </div>

      {/* Help Text */}
      <div className="mt-4 text-xs text-gray-500 dark:text-gray-400 text-center">
        Expected wins are calculated by simulating each game thousands of times
        based on scoring performance. Win Luck = Actual Wins - Expected Wins.
      </div>
    </div>
  );
}

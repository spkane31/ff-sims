import React from "react";
import { Matchup } from "../types/models";
import { calculateLeagueStats } from "../utils/expectedWinsCalculations";

interface Team {
  id: number;
  name: string;
  ownerName: string;
}

interface ExpectedWinsBannerProps {
  matchups: Matchup[];
  teams: Team[];
  isLoading: boolean;
  currentYear: number;
}

// TODO seankane: Is this used anywhere? If not, delete it.
export default function ExpectedWinsBanner({
  matchups,
  teams,
  isLoading,
  currentYear,
}: ExpectedWinsBannerProps) {
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

  const teamIds = teams.map((t) => t.id);
  const stats = calculateLeagueStats(matchups, teamIds);

  if (teams.length === 0) {
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

  // Find team names for luckiest/unluckiest
  const luckiestTeam = stats.luckiestTeam
    ? teams.find((t) => t.id === stats.luckiestTeam!.teamId)
    : null;
  const unluckiestTeam = stats.unluckiestTeam
    ? teams.find((t) => t.id === stats.unluckiestTeam!.teamId)
    : null;

  return (
    <div className="bg-gradient-to-r from-blue-50 to-purple-50 dark:from-blue-900/20 dark:to-purple-900/20 p-6 rounded-lg shadow-md mb-8">
      <div className="text-center mb-6">
        <h2 className="text-2xl font-bold text-gray-800 dark:text-gray-200 mb-2">
          Expected Wins Analysis - {currentYear}
        </h2>
        <p className="text-gray-600 dark:text-gray-400">
          How many wins each team &ldquo;should&rdquo; have based on their
          scoring performance
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
          <div
            className={`text-3xl font-bold ${
              stats.averageWinLuck > 0
                ? "text-green-600 dark:text-green-400"
                : stats.averageWinLuck < 0
                ? "text-red-600 dark:text-red-400"
                : "text-gray-600 dark:text-gray-400"
            }`}
          >
            {stats.averageWinLuck > 0 ? "+" : ""}
            {stats.averageWinLuck.toFixed(1)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
            Average Win Luck
          </div>
        </div>

        {luckiestTeam && stats.luckiestTeam && (
          <div className="text-center">
            <div className="text-2xl font-bold text-green-600 dark:text-green-400">
              +{stats.luckiestTeam.luck.toFixed(1)}
            </div>
            <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
              Luckiest Team
            </div>
            <div className="text-xs font-medium text-gray-700 dark:text-gray-300">
              {luckiestTeam.ownerName || luckiestTeam.name}
            </div>
          </div>
        )}

        {unluckiestTeam && stats.unluckiestTeam && (
          <div className="text-center">
            <div className="text-2xl font-bold text-red-600 dark:text-red-400">
              {stats.unluckiestTeam.luck.toFixed(1)}
            </div>
            <div className="text-sm text-gray-600 dark:text-gray-400 mt-1">
              Unluckiest Team
            </div>
            <div className="text-xs font-medium text-gray-700 dark:text-gray-300">
              {unluckiestTeam.ownerName || unluckiestTeam.name}
            </div>
          </div>
        )}
      </div>

      <div className="mt-6 text-center">
        <p className="text-xs text-gray-500 dark:text-gray-400">
          Win Luck = Actual Wins - Expected Wins. Positive values indicate teams
          that have been lucky with their schedule.
        </p>
      </div>
    </div>
  );
}

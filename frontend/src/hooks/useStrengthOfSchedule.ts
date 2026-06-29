import { useMemo } from "react";
import { Matchup } from "@/types/models";

export interface TeamStrength {
  team: string;
  difficulty: "Easy" | "Med" | "Hard";
  strengthPercentage: number;
}

export interface StrengthOfScheduleResult {
  overallStrength: TeamStrength[];
  remainingStrength: TeamStrength[];
}

/**
 * Custom hook to calculate strength of schedule metrics
 * @param scheduleData - Array of matchup data
 * @param targetYear - Year to calculate SOS for (defaults to current year)
 * @returns Object containing overall and remaining strength of schedule data
 */
export function useStrengthOfSchedule(
  scheduleData: Matchup[] | undefined,
  targetYear?: number
): StrengthOfScheduleResult {
  return useMemo(() => {
    if (!scheduleData || scheduleData.length === 0) {
      return { overallStrength: [], remainingStrength: [] };
    }

    // Use provided year or get the latest year from the data
    const year = targetYear ?? Math.max(...scheduleData.map(g => g.year));

    // Filter to regular season games only for the target year
    const regularSeasonGames = scheduleData.filter(
      g => g.year === year && !g.isPlayoff && g.gameType === "NONE"
    );

    if (regularSeasonGames.length === 0) {
      return { overallStrength: [], remainingStrength: [] };
    }

    // Get all unique teams
    const teams = new Set<string>();
    regularSeasonGames.forEach(game => {
      teams.add(game.homeTeamName);
      teams.add(game.awayTeamName);
    });

    // Calculate win percentage for each team from completed games
    const teamWinPct: Record<string, { wins: number; losses: number; pct: number }> = {};
    teams.forEach(team => {
      const completedGames = regularSeasonGames.filter(
        g => g.completed && (g.homeTeamName === team || g.awayTeamName === team)
      );

      let wins = 0;
      let losses = 0;

      completedGames.forEach(game => {
        if (game.homeTeamName === team) {
          if (game.homeScore > game.awayScore) wins++;
          else if (game.homeScore < game.awayScore) losses++;
        } else {
          if (game.awayScore > game.homeScore) wins++;
          else if (game.awayScore < game.homeScore) losses++;
        }
      });

      const total = wins + losses;
      teamWinPct[team] = {
        wins,
        losses,
        pct: total > 0 ? wins / total : 0.5 // Default to 0.5 if no games played
      };
    });

    // Calculate SOS for each team - average opponent win percentage
    const calculateSOS = (games: Matchup[]) => {
      const teamSOS: Record<string, { totalOppPct: number; oppCount: number }> = {};

      games.forEach(game => {
        const homeTeam = game.homeTeamName;
        const awayTeam = game.awayTeamName;

        // Initialize
        if (!teamSOS[homeTeam]) teamSOS[homeTeam] = { totalOppPct: 0, oppCount: 0 };
        if (!teamSOS[awayTeam]) teamSOS[awayTeam] = { totalOppPct: 0, oppCount: 0 };

        // Add opponent's win percentage
        teamSOS[homeTeam].totalOppPct += teamWinPct[awayTeam]?.pct ?? 0.5;
        teamSOS[homeTeam].oppCount++;

        teamSOS[awayTeam].totalOppPct += teamWinPct[homeTeam]?.pct ?? 0.5;
        teamSOS[awayTeam].oppCount++;
      });

      return teamSOS;
    };

    // Calculate overall SOS (all games)
    const overallSOS = calculateSOS(regularSeasonGames);

    // Calculate remaining SOS (future games only)
    const futureGames = regularSeasonGames.filter(g => !g.completed);
    const remainingSOS = calculateSOS(futureGames);

    // Convert to display format
    const convertToStrength = (sosData: Record<string, { totalOppPct: number; oppCount: number }>): TeamStrength[] => {
      return Object.entries(sosData)
        .map(([team, data]) => {
          const avgOppPct = data.oppCount > 0 ? data.totalOppPct / data.oppCount : 0.5;
          const strengthPercentage = Math.round(avgOppPct * 100);

          let difficulty: "Easy" | "Med" | "Hard";
          if (strengthPercentage >= 55) {
            difficulty = "Hard";
          } else if (strengthPercentage >= 45) {
            difficulty = "Med";
          } else {
            difficulty = "Easy";
          }

          return { team, difficulty, strengthPercentage };
        })
        .sort((a, b) => b.strengthPercentage - a.strengthPercentage);
    };

    return {
      overallStrength: convertToStrength(overallSOS),
      remainingStrength: convertToStrength(remainingSOS)
    };
  }, [scheduleData, targetYear]);
}

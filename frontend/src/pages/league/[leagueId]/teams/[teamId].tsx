import { useState, useEffect } from "react";
import { useRouter } from "next/router";
import Link from "next/link";
import {
  teamsService,
  TeamDetail as TeamDetailType,
} from "@/services/teamsService";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import EmptyState from "@/components/design-system/EmptyState";
import ErrorState from "@/components/design-system/ErrorState";
import DataTable, {
  type DataTableColumn,
} from "@/components/design-system/DataTable";

// Type definitions for the legacy UI components
interface Player {
  id: number | string;
  name: string;
  position: string;
  team: string;
  points: number;
  projection: number;
  status: string;
}

interface DraftPick {
  round: number;
  pick: number;
  overall: number; // Calculated based on round and pick
  description: string;
  playerId: number | string; // Reference to associated player
  player: string;
  position: string;
}

interface Game {
  week: number;
  year: number;
  opponent: string;
  opponentESPNID: string; // Add opponent ESPN ID for linking
  result: "W" | "L" | "T" | "-";
  score: string;
  isHome: boolean;
  isPlayoff?: boolean; // Add isPlayoff field
  matchupId?: string; // Add matchup ID for linking to schedule detail page
}

type Tab = "overview" | "players" | "schedule" | "draft" | "transactions";

// This mapping function converts API data to UI component format
function mapApiDataToUiFormat(teamData: TeamDetailType): {
  id: number;
  name: string;
  owner: string;
  record: {
    wins: number;
    losses: number;
    ties: number;
  };
  points: {
    scored: number;
    against: number;
  };
  playoffChance: number; // TODO: Calculate from simulation data
  players: Player[];
  draftPicks: DraftPick[];
  schedule: Game[];
  transactions: {
    id: string;
    type: string;
    date: string;
    year: number;
    week: number;
    description: string;
    playersGained: {
      id: string;
      name: string;
    }[];
    playersLost: {
      id: string;
      name: string;
    }[];
  }[];
} {
  // Convert API players to UI format
  const players: Player[] = teamData.currentPlayers.map((player) => ({
    id: player.id,
    name: player.name,
    position: player.position,
    team: player.team,
    points: player.fantasyPoints,
    projection: 0, // TODO: Get projections from API
    status: player.status,
  }));

  // Convert API draft picks to UI format if draftPicks is not null
  const draftPicks: DraftPick[] = teamData.draftPicks
    ? teamData.draftPicks.map((pick) => ({
        round: pick.round,
        pick: pick.pick,
        overall: (pick.round - 1) * 12 + pick.pick, // Assuming 12 teams per round
        description: `${pick.round}${getOrdinalSuffix(pick.round)} Round (${
          pick.year
        })`,
        playerId: 0, // TODO: Link draft picks to players when API supports it
        player: pick.player,
        position: pick.position,
      }))
    : [];

  let pointsScored = 0;
  let pointsAgainst = 0;
  teamData.schedule.forEach((game) => {
    if (game.completed) {
      pointsScored += game.teamScore;
      pointsAgainst += game.opponentScore;
    }
  });

  let wins = 0;
  let losses = 0;
  let ties = 0;
  teamData.schedule.forEach((game) => {
    if (game.completed) {
      if (game.result === "W") wins++;
      else if (game.result === "L") losses++;
      else if (game.result === "T") ties++;
    }
  });

  // Convert API schedule to UI format
  const schedule: Game[] = teamData.schedule.map((game) => ({
    week: game.week,
    year: game.year,
    opponent: game.opponent,
    result: game.result as "W" | "L" | "T" | "-",
    score: game.completed
      ? `${game.teamScore.toFixed(2)}-${game.opponentScore.toFixed(2)}`
      : "0-0",
    isHome: game.isHome,
    opponentESPNID: game.opponentESPNID, // Add opponent ESPN ID
    isPlayoff: game.isPlayoff, // Add isPlayoff field
    matchupId: game.matchupId, // Add matchup ID for linking
  }));

  // Pass through the transactions directly
  const transactions = teamData.transactions || [];

  return {
    id: parseInt(teamData.id),
    name: teamData.name,
    owner: teamData.owner,
    record: {
      wins,
      losses,
      ties,
    },
    points: {
      scored: pointsScored,
      against: pointsAgainst,
    },
    playoffChance: 0, // TODO: Calculate from simulation data
    players,
    draftPicks,
    schedule,
    transactions,
  };
}

// Helper function to get ordinal suffix (1st, 2nd, 3rd, etc.)
function getOrdinalSuffix(num: number): string {
  const j = num % 10;
  const k = num % 100;
  if (j === 1 && k !== 11) {
    return "st";
  }
  if (j === 2 && k !== 12) {
    return "nd";
  }
  if (j === 3 && k !== 13) {
    return "rd";
  }
  return "th";
}

// Calculate year-by-year team records
function calculateYearByYearRecords(games: Game[]) {
  const yearRecords = new Map<
    number,
    {
      year: number;
      regularSeason: { wins: number; losses: number; ties: number };
      playoffs: { wins: number; losses: number; ties: number };
      totalPoints: number;
      gamesPlayed: number;
    }
  >();

  games.forEach((game) => {
    if (game.result === "-") return; // Skip upcoming games

    if (!yearRecords.has(game.year)) {
      yearRecords.set(game.year, {
        year: game.year,
        regularSeason: { wins: 0, losses: 0, ties: 0 },
        playoffs: { wins: 0, losses: 0, ties: 0 },
        totalPoints: 0,
        gamesPlayed: 0,
      });
    }

    const record = yearRecords.get(game.year)!;

    // Parse score to get points
    const scoreParts = game.score.split("-");
    const teamPoints = parseFloat(scoreParts[0]) || 0;

    record.totalPoints += teamPoints;
    record.gamesPlayed++;

    // Check if this is a playoff game - look for isPlayoff field or fall back to week logic
    const isPlayoffWeek =
      game.isPlayoff !== undefined ? game.isPlayoff : game.week > 14;

    if (isPlayoffWeek) {
      if (game.result === "W") record.playoffs.wins++;
      else if (game.result === "L") record.playoffs.losses++;
      else if (game.result === "T") record.playoffs.ties++;
    } else {
      if (game.result === "W") record.regularSeason.wins++;
      else if (game.result === "L") record.regularSeason.losses++;
      else if (game.result === "T") record.regularSeason.ties++;
    }
  });

  return Array.from(yearRecords.values()).sort((a, b) => b.year - a.year);
}

type YearRecord = ReturnType<typeof calculateYearByYearRecords>[number];

// Calculate team performance metrics from schedule data
function calculateTeamStats(games: Game[]) {
  // Filter only completed games (games with results)
  const completedGames = games.filter((game) => game.result !== "-");
  if (completedGames.length === 0) return null;

  // Current streak calculation
  const currentStreakGames = [...completedGames].sort((a, b) => {
    if (a.year !== b.year) return b.year - a.year;
    return b.week - a.week;
  });

  const streakType = currentStreakGames[0]?.result;
  let streakCount = 0;

  for (const game of currentStreakGames) {
    if (game.result === streakType) {
      streakCount++;
    } else {
      break;
    }
  }

  // Home vs Away performance
  const homeGames = completedGames.filter((game) => game.isHome);
  const awayGames = completedGames.filter((game) => !game.isHome);

  const homeRecord = {
    wins: homeGames.filter((game) => game.result === "W").length,
    losses: homeGames.filter((game) => game.result === "L").length,
    ties: homeGames.filter((game) => game.result === "T").length,
  };

  const awayRecord = {
    wins: awayGames.filter((game) => game.result === "W").length,
    losses: awayGames.filter((game) => game.result === "L").length,
    ties: awayGames.filter((game) => game.result === "T").length,
  };

  // Recent form (last 5 games)
  const recentGames = [...completedGames]
    .sort((a, b) => {
      if (a.year !== b.year) return b.year - a.year;
      return b.week - a.week;
    })
    .slice(0, 5);

  const recentForm = recentGames.map((game) => game.result);

  // Opponent analysis - find most common opponents and record against them
  const opponentStats: Record<
    string,
    { wins: number; losses: number; ties: number; opponentESPNID: string }
  > = {};

  completedGames.forEach((game) => {
    if (!opponentStats[game.opponent]) {
      opponentStats[game.opponent] = {
        wins: 0,
        losses: 0,
        ties: 0,
        opponentESPNID: game.opponentESPNID,
      };
    }

    if (game.result === "W") opponentStats[game.opponent].wins++;
    else if (game.result === "L") opponentStats[game.opponent].losses++;
    else if (game.result === "T") opponentStats[game.opponent].ties++;
  });

  // Get top 3 most played against opponents
  const topOpponents = Object.entries(opponentStats)
    .map(([opponent, record]) => ({
      opponent,
      opponentESPNID: record.opponentESPNID, // Include opponent ESPN ID for linking
      totalGames: record.wins + record.losses + record.ties,
      winPercentage:
        (record.wins / (record.wins + record.losses + record.ties)) * 100,
      record,
    }))
    .sort((a, b) => b.totalGames - a.totalGames)
    .slice(0, 3);

  return {
    streak: {
      type: streakType,
      count: streakCount,
    },
    homeRecord,
    awayRecord,
    recentForm,
    topOpponents,
  };
}

// Helper to get formatted streak text
function getStreakText(streak: { type: string; count: number }) {
  if (!streak || streak.count === 0) return "No streak";

  const streakType =
    streak.type === "W" ? "Win" : streak.type === "L" ? "Loss" : "Tie";
  return `${streakType} ${streak.count}`;
}

// A game result (and a win/loss streak) is the outcome of a comparison, so it
// maps to the semantic --status-* tokens rather than the categorical chart
// palette. This is the exact mapping the page used before (green W / red L /
// yellow otherwise), shared by the streak label, the recent-form circles, the
// recent-performance circles and the schedule-card circles.
function resultTokens(result: string): { fg: string; bg: string } {
  if (result === "W") {
    return { fg: "var(--status-success-fg)", bg: "var(--status-success-bg)" };
  }
  if (result === "L") {
    return { fg: "var(--status-danger-fg)", bg: "var(--status-danger-bg)" };
  }
  return { fg: "var(--status-warning-fg)", bg: "var(--status-warning-bg)" };
}

// A winning record is a positive result, a losing record a negative one, an
// even record neither — same three-way comparison the year-by-year table used
// for both its regular-season and playoff record cells.
function recordColor(wins: number, losses: number): string {
  if (wins > losses) return "var(--status-success-fg)";
  if (wins < losses) return "var(--status-danger-fg)";
  return "var(--status-warning-fg)";
}

// Roster availability is a status (available / out / questionable-ish), so the
// semantic status tokens apply.
function playerStatusTokens(status: string): { fg: string; bg: string } {
  if (status === "Active") {
    return { fg: "var(--status-success-fg)", bg: "var(--status-success-bg)" };
  }
  if (status === "IR") {
    return { fg: "var(--status-danger-fg)", bg: "var(--status-danger-bg)" };
  }
  return { fg: "var(--status-warning-fg)", bg: "var(--status-warning-bg)" };
}

// Transaction type is an unordered category label (draft vs. trade vs. the
// rest), not a status, so it uses the colorblind-safe qualitative palette. The
// hues mirror the ones this page used before (purple draft / blue trade / green
// other) and agree with the trade colour on league/[leagueId]/transactions.
function txTypeColor(type: string): string {
  const normalized = type.toLowerCase();
  if (normalized === "draft") return "var(--chart-series-4)";
  if (normalized === "trade") return "var(--chart-series-1)";
  return "var(--chart-series-3)";
}

type IndexedDraftPick = { pick: DraftPick; index: number };

export default function TeamDetail() {
  const router = useRouter();
  const { teamId, leagueId } = router.query;
  const leagueIdNum = Number(leagueId);

  const [isLoading, setIsLoading] = useState(true);
  const [team, setTeam] = useState<ReturnType<
    typeof mapApiDataToUiFormat
  > | null>(null);
  const [activeTab, setActiveTab] = useState<Tab>("overview");
  const [, setError] = useState<string | null>(null);

  // Add these state variables at the top of the TeamDetail function component
  const [yearFilter, setYearFilter] = useState<string>("all");
  const [opponentFilter, setOpponentFilter] = useState<string>("all");
  const [transactionYearFilter, setTransactionYearFilter] =
    useState<string>("all");

  // Add these state variables
  const [teamStats, setTeamStats] = useState<ReturnType<
    typeof calculateTeamStats
  > | null>(null);

  // Add state for year-by-year records
  const [yearByYearRecords, setYearByYearRecords] = useState<
    ReturnType<typeof calculateYearByYearRecords>
  >([]);

  useEffect(() => {
    if (!teamId) return;

    const fetchTeamData = async () => {
      try {
        setIsLoading(true);
        setError(null);

        // Use teamsService to fetch detailed team data
        const teamData = await teamsService.getTeamDetail(leagueIdNum, teamId as string);

        // Map API data to the format expected by the UI
        const mappedTeam = mapApiDataToUiFormat(teamData);
        setTeam(mappedTeam);

        // Calculate additional team statistics
        if (mappedTeam) {
          const stats = calculateTeamStats(mappedTeam.schedule);
          setTeamStats(stats);

          // Calculate year-by-year records
          const yearRecords = calculateYearByYearRecords(mappedTeam.schedule);
          setYearByYearRecords(yearRecords);
        }
      } catch (err) {
        console.error("Error fetching team data:", err);
        setError(
          err instanceof Error ? err.message : "An unknown error occurred"
        );
      } finally {
        setIsLoading(false);
      }
    };

    fetchTeamData();
  }, [teamId]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Skeleton className="w-8 h-8 rounded-full" />
        <span className="ml-3 text-lg" style={{ color: "var(--text-primary)" }}>
          Loading team data...
        </span>
      </div>
    );
  }

  if (!team) {
    return (
      <div className="space-y-4">
        <h2
          className="text-lg font-medium"
          style={{ color: "var(--text-primary)" }}
        >
          Team not found
        </h2>
        <ErrorState message="We could not find a team with the requested ID. Please check the URL and try again." />
        <Link
          href={`/league/${leagueIdNum}/teams`}
          className="inline-block hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          ← Back to Teams
        </Link>
      </div>
    );
  }

  // Schedule tab: games matching the year + opponent filters (same filter chain
  // the grid and the "no games" message each ran before).
  const filteredSchedule = team.schedule
    .filter(
      (game) => yearFilter === "all" || game.year.toString() === yearFilter
    )
    .filter(
      (game) => opponentFilter === "all" || game.opponent === opponentFilter
    );

  // Draft tab: picks matching the year filter (shares the transaction year
  // filter state, exactly as before).
  const filteredDraftPicks: IndexedDraftPick[] = team.draftPicks
    .filter((pick) => {
      if (transactionYearFilter === "all") return true;
      const pickYear = pick.description.match(/\((\d{4})\)/)?.[1];
      return pickYear === transactionYearFilter;
    })
    .map((pick, index) => ({ pick, index }));

  const yearRecordColumns: DataTableColumn<YearRecord>[] = [
    {
      id: "year",
      header: "Year",
      cell: (yearRecord) => (
        <span
          className="font-medium"
          style={{ color: "var(--text-primary)" }}
        >
          {yearRecord.year}
        </span>
      ),
    },
    {
      id: "regularSeason",
      header: "Regular Season",
      cell: (yearRecord) => (
        <span
          className="font-medium"
          style={{
            color: recordColor(
              yearRecord.regularSeason.wins,
              yearRecord.regularSeason.losses
            ),
          }}
        >
          {`${yearRecord.regularSeason.wins}-${
            yearRecord.regularSeason.losses
          }${
            yearRecord.regularSeason.ties > 0
              ? `-${yearRecord.regularSeason.ties}`
              : ""
          }`}
        </span>
      ),
    },
    {
      id: "playoffs",
      header: "Playoff Record",
      cell: (yearRecord) => {
        const playoffRecord = `${yearRecord.playoffs.wins}-${
          yearRecord.playoffs.losses
        }${
          yearRecord.playoffs.ties > 0 ? `-${yearRecord.playoffs.ties}` : ""
        }`;
        const totalPlayoffGames =
          yearRecord.playoffs.wins +
          yearRecord.playoffs.losses +
          yearRecord.playoffs.ties;

        return totalPlayoffGames > 0 ? (
          <span
            className="font-medium"
            style={{
              color: recordColor(
                yearRecord.playoffs.wins,
                yearRecord.playoffs.losses
              ),
            }}
          >
            {playoffRecord}
          </span>
        ) : (
          <span style={{ color: "var(--text-muted)" }}>-</span>
        );
      },
    },
    {
      id: "totalPoints",
      header: "Total Points",
      cell: (yearRecord) => (
        <span className="font-medium" style={{ color: "var(--text-primary)" }}>
          {yearRecord.totalPoints.toFixed(2)}
        </span>
      ),
    },
    {
      id: "avgPoints",
      header: "Avg Points/Game",
      cell: (yearRecord) => (
        <span style={{ color: "var(--text-secondary)" }}>
          {yearRecord.gamesPlayed > 0
            ? (yearRecord.totalPoints / yearRecord.gamesPlayed).toFixed(2)
            : "-"}
        </span>
      ),
    },
  ];

  const rosterColumns: DataTableColumn<Player>[] = [
    {
      id: "position",
      header: "Position",
      cell: (player) => (
        <span style={{ color: "var(--text-primary)" }}>{player.position}</span>
      ),
    },
    {
      id: "name",
      header: "Player",
      cell: (player) => (
        <span className="font-medium" style={{ color: "var(--text-primary)" }}>
          {player.name}
        </span>
      ),
    },
    {
      id: "team",
      header: "Team",
      cell: (player) => (
        <span style={{ color: "var(--text-primary)" }}>{player.team}</span>
      ),
    },
    {
      id: "points",
      header: "Points",
      cell: (player) => (
        <span style={{ color: "var(--text-primary)" }}>
          {player.points.toFixed(2)}
        </span>
      ),
    },
    {
      id: "projection",
      header: "Projection",
      cell: (player) => (
        <span style={{ color: "var(--text-primary)" }}>
          {player.projection > 0 ? player.projection.toFixed(2) : "-"}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (player) => {
        const tokens = playerStatusTokens(player.status);
        return (
          <Badge
            variant="outline"
            className="rounded-full"
            style={{
              color: tokens.fg,
              borderColor: tokens.fg,
              backgroundColor: tokens.bg,
            }}
          >
            {player.status}
          </Badge>
        );
      },
    },
  ];

  const draftPickColumns: DataTableColumn<IndexedDraftPick>[] = [
    {
      id: "year",
      header: "Year",
      cell: ({ pick }) => (
        <span style={{ color: "var(--text-primary)" }}>
          {pick.description.match(/\((\d{4})\)/)?.[1] || ""}
        </span>
      ),
    },
    {
      id: "round",
      header: "Round",
      cell: ({ pick }) => (
        <span style={{ color: "var(--text-primary)" }}>{pick.round}</span>
      ),
    },
    {
      id: "pick",
      header: "Pick",
      cell: ({ pick }) => (
        <span style={{ color: "var(--text-primary)" }}>{pick.pick}</span>
      ),
    },
    {
      id: "overall",
      header: "Overall",
      cell: ({ pick }) => (
        <span style={{ color: "var(--text-primary)" }}>{pick.overall}</span>
      ),
    },
    {
      id: "player",
      header: "Player",
      cell: ({ pick }) => (
        <Link
          href="#"
          onClick={(e) => e.preventDefault()}
          className="hover:underline"
          style={{ color: "var(--action-primary)" }}
        >
          {pick.player} ({pick.position})
        </Link>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      {/* Team Header */}
      <Card>
        <CardContent className="p-6">
          <div className="flex flex-col md:flex-row justify-between md:items-center">
            <div>
              <div className="flex items-center">
                <h1
                  className="text-3xl md:text-4xl font-bold"
                  style={{ color: "var(--action-primary)" }}
                >
                  {team.name}
                </h1>
              </div>
              <p className="text-lg" style={{ color: "var(--text-muted)" }}>
                Managed by {team.owner}
              </p>
            </div>

            <div className="mt-4 md:mt-0">
              <Link
                href={`/league/${leagueIdNum}/teams`}
                className="hover:underline"
                style={{ color: "var(--action-primary)" }}
              >
                ← Back to Teams
              </Link>
            </div>
          </div>
        </CardContent>
      </Card>

      <Tabs
        value={activeTab}
        onValueChange={(value) => setActiveTab(value as Tab)}
      >
        {/* Navigation Tabs */}
        <TabsList
          variant="line"
          className="mb-2 max-w-full overflow-x-auto scrollbar-hide"
        >
          <TabsTrigger value="overview" className="px-3">
            Overview
          </TabsTrigger>
          <TabsTrigger value="players" className="px-3">
            Players
          </TabsTrigger>
          <TabsTrigger value="schedule" className="px-3">
            Schedule
          </TabsTrigger>
          <TabsTrigger value="draft" className="px-3">
            Draft Picks
          </TabsTrigger>
          <TabsTrigger value="transactions" className="px-3">
            Transactions
          </TabsTrigger>
        </TabsList>

        {/* Overview Tab */}
        <TabsContent value="overview" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2
                className="text-xl font-semibold mb-4"
                style={{ color: "var(--text-primary)" }}
              >
                Team Overview
              </h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
                <div
                  className="p-4 rounded-lg"
                  style={{ backgroundColor: "var(--surface-sunken)" }}
                >
                  <h3
                    className="text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Overall Record
                  </h3>
                  <div
                    className="text-2xl font-bold"
                    style={{ color: "var(--text-primary)" }}
                  >
                    {teamStats
                      ? teamStats.awayRecord.wins + teamStats.homeRecord.wins
                      : "-"}
                    -
                    {teamStats
                      ? teamStats.awayRecord.losses +
                        teamStats.homeRecord.losses
                      : "-"}
                    {teamStats
                      ? teamStats.awayRecord.ties +
                          teamStats.homeRecord.ties >
                        0
                        ? teamStats.awayRecord.ties +
                          teamStats.homeRecord.ties
                        : ""
                      : ""}
                  </div>
                  {teamStats?.streak && (
                    <div className="mt-1 text-sm">
                      <span
                        className="font-medium"
                        style={{
                          color: resultTokens(teamStats.streak.type).fg,
                        }}
                      >
                        {getStreakText(teamStats.streak)}
                      </span>
                    </div>
                  )}
                </div>

                <div
                  className="p-4 rounded-lg"
                  style={{ backgroundColor: "var(--surface-sunken)" }}
                >
                  <h3
                    className="text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Points For
                  </h3>
                  <div
                    className="text-2xl font-bold"
                    style={{ color: "var(--text-primary)" }}
                  >
                    {team.points.scored.toLocaleString(undefined, {
                      minimumFractionDigits: 2,
                      maximumFractionDigits: 2,
                    })}
                  </div>
                  <div
                    className="mt-1 text-sm"
                    style={{ color: "var(--text-muted)" }}
                  >
                    {(
                      team.points.scored /
                      (team.record.wins +
                        team.record.losses +
                        team.record.ties)
                    ).toFixed(2)}{" "}
                    per game
                  </div>
                </div>

                <div
                  className="p-4 rounded-lg"
                  style={{ backgroundColor: "var(--surface-sunken)" }}
                >
                  <h3
                    className="text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Points Against
                  </h3>
                  <div
                    className="text-2xl font-bold"
                    style={{ color: "var(--text-primary)" }}
                  >
                    {team.points.against.toLocaleString(undefined, {
                      minimumFractionDigits: 2,
                      maximumFractionDigits: 2,
                    })}
                  </div>
                  <div
                    className="mt-1 text-sm"
                    style={{ color: "var(--text-muted)" }}
                  >
                    {(
                      team.points.against /
                      (team.record.wins +
                        team.record.losses +
                        team.record.ties)
                    ).toFixed(2)}{" "}
                    per game
                  </div>
                </div>

                <div
                  className="p-4 rounded-lg"
                  style={{ backgroundColor: "var(--surface-sunken)" }}
                >
                  <h3
                    className="text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Point Differential
                  </h3>
                  <div
                    className="text-2xl font-bold"
                    style={{
                      color:
                        team.points.scored - team.points.against > 0
                          ? "var(--status-success-fg)"
                          : "var(--status-danger-fg)",
                    }}
                  >
                    {team.points.scored - team.points.against > 0 ? "+" : ""}
                    {(team.points.scored - team.points.against).toFixed(2)}
                  </div>
                  <div
                    className="mt-1 text-sm"
                    style={{ color: "var(--text-muted)" }}
                  >
                    {(
                      (team.points.scored - team.points.against) /
                      (team.record.wins +
                        team.record.losses +
                        team.record.ties)
                    ).toFixed(2)}{" "}
                    per game
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Add detailed performance metrics section */}
          <Card>
            <CardContent className="p-6">
              <h2
                className="text-xl font-semibold mb-4"
                style={{ color: "var(--text-primary)" }}
              >
                Performance Breakdown
              </h2>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="border rounded-lg p-4">
                  <h3
                    className="font-medium mb-3 text-lg"
                    style={{ color: "var(--text-primary)" }}
                  >
                    Home vs Away
                  </h3>
                  <div className="flex flex-col space-y-4">
                    <div>
                      <div className="flex justify-between mb-1">
                        <span style={{ color: "var(--text-primary)" }}>
                          Home Record
                        </span>
                        <span
                          className="font-medium"
                          style={{ color: "var(--text-primary)" }}
                        >
                          {teamStats?.homeRecord
                            ? `${teamStats.homeRecord.wins}-${
                                teamStats.homeRecord.losses
                              }${
                                teamStats.homeRecord.ties > 0
                                  ? `-${teamStats.homeRecord.ties}`
                                  : ""
                              }`
                            : "N/A"}
                        </span>
                      </div>
                      {teamStats?.homeRecord && (
                        <div
                          className="w-full rounded-full h-2.5"
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                          }}
                        >
                          <div
                            className="h-2.5 rounded-full"
                            style={{
                              backgroundColor: "var(--chart-series-1)",
                              width: `${
                                (teamStats.homeRecord.wins /
                                  (teamStats.homeRecord.wins +
                                    teamStats.homeRecord.losses +
                                    teamStats.homeRecord.ties)) *
                                100
                              }%`,
                            }}
                          ></div>
                        </div>
                      )}
                    </div>

                    <div>
                      <div className="flex justify-between mb-1">
                        <span style={{ color: "var(--text-primary)" }}>
                          Away Record
                        </span>
                        <span
                          className="font-medium"
                          style={{ color: "var(--text-primary)" }}
                        >
                          {teamStats?.awayRecord
                            ? `${teamStats.awayRecord.wins}-${
                                teamStats.awayRecord.losses
                              }${
                                teamStats.awayRecord.ties > 0
                                  ? `-${teamStats.awayRecord.ties}`
                                  : ""
                              }`
                            : "N/A"}
                        </span>
                      </div>
                      {teamStats?.awayRecord && (
                        <div
                          className="w-full rounded-full h-2.5"
                          style={{
                            backgroundColor: "var(--surface-sunken)",
                          }}
                        >
                          <div
                            className="h-2.5 rounded-full"
                            style={{
                              backgroundColor: "var(--chart-series-3)",
                              width: `${
                                (teamStats.awayRecord.wins /
                                  (teamStats.awayRecord.wins +
                                    teamStats.awayRecord.losses +
                                    teamStats.awayRecord.ties)) *
                                100
                              }%`,
                            }}
                          ></div>
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                <div className="border rounded-lg p-4">
                  <h3
                    className="font-medium mb-3 text-lg"
                    style={{ color: "var(--text-primary)" }}
                  >
                    Recent Form
                  </h3>
                  {teamStats?.recentForm &&
                  teamStats.recentForm.length > 0 ? (
                    <div className="flex space-x-2 mb-2">
                      {teamStats.recentForm.map((result, i) => (
                        <div
                          key={i}
                          className="w-8 h-8 rounded-full flex items-center justify-center font-medium"
                          style={{
                            backgroundColor: resultTokens(result).fg,
                            color: resultTokens(result).bg,
                          }}
                        >
                          {result}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <p style={{ color: "var(--text-muted)" }}>
                      No recent games available
                    </p>
                  )}

                  <div className="mt-4">
                    <h4
                      className="text-sm font-medium mb-1"
                      style={{ color: "var(--text-muted)" }}
                    >
                      Last 5 Games
                    </h4>
                    <div
                      className="text-sm"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {teamStats?.recentForm && (
                        <span>
                          {
                            teamStats.recentForm.filter((r) => r === "W")
                              .length
                          }{" "}
                          wins,{" "}
                          {
                            teamStats.recentForm.filter((r) => r === "L")
                              .length
                          }{" "}
                          losses
                          {teamStats.recentForm.filter((r) => r === "T")
                            .length > 0 &&
                            `, ${
                              teamStats.recentForm.filter((r) => r === "T")
                                .length
                            } ties`}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          <section className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Card>
              <CardContent className="p-6">
                <h2
                  className="text-lg font-semibold mb-4"
                  style={{ color: "var(--text-primary)" }}
                >
                  Playoff Odds
                </h2>
                {/* TODO(seankane): this whole section is hard coded */}
                <div className="space-y-4">
                  <div>
                    <div className="flex justify-between mb-1">
                      <span style={{ color: "var(--text-primary)" }}>
                        Last Place
                      </span>
                      <span
                        className="font-medium"
                        style={{ color: "var(--text-primary)" }}
                      >
                        0%
                      </span>
                    </div>
                    <div
                      className="w-full rounded-full h-2.5"
                      style={{ backgroundColor: "var(--surface-sunken)" }}
                    >
                      <div
                        className="h-2.5 rounded-full"
                        style={{
                          backgroundColor: "var(--action-primary)",
                          width: `${team.playoffChance}%`,
                        }}
                      ></div>
                    </div>
                  </div>

                  <div>
                    <div className="flex justify-between mb-1">
                      <span style={{ color: "var(--text-primary)" }}>
                        Make Playoffs
                      </span>
                      <span
                        className="font-medium"
                        style={{ color: "var(--text-primary)" }}
                      >
                        {team.playoffChance}%
                      </span>
                    </div>
                    <div
                      className="w-full rounded-full h-2.5"
                      style={{ backgroundColor: "var(--surface-sunken)" }}
                    >
                      <div
                        className="h-2.5 rounded-full"
                        style={{
                          backgroundColor: "var(--action-primary)",
                          width: `${team.playoffChance}%`,
                        }}
                      ></div>
                    </div>
                  </div>

                  <div>
                    <div className="flex justify-between mb-1">
                      <span style={{ color: "var(--text-primary)" }}>
                        Win Championship
                      </span>
                      <span
                        className="font-medium"
                        style={{ color: "var(--text-primary)" }}
                      >
                        0%
                      </span>
                    </div>
                    <div
                      className="w-full rounded-full h-2.5"
                      style={{ backgroundColor: "var(--surface-sunken)" }}
                    >
                      <div
                        className="h-2.5 rounded-full"
                        style={{
                          backgroundColor: "var(--chart-series-2)",
                          width: "0%",
                        }}
                      ></div>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardContent className="p-6">
                <h2
                  className="text-lg font-semibold mb-4"
                  style={{ color: "var(--text-primary)" }}
                >
                  Recent Performance
                </h2>
                <div className="space-y-3">
                  {team.schedule
                    .slice(-5)
                    .reverse()
                    .map((game, i) => (
                      <div
                        key={i}
                        className="flex items-center justify-between py-2 border-b last:border-0"
                      >
                        <div>
                          <span
                            className="font-medium"
                            style={{ color: "var(--text-primary)" }}
                          >
                            Week {game.week}
                          </span>
                          <span
                            className="mx-2"
                            style={{ color: "var(--text-muted)" }}
                          >
                            vs
                          </span>
                          <span style={{ color: "var(--text-primary)" }}>
                            {game.opponent}
                          </span>
                        </div>
                        <div className="flex items-center">
                          <span
                            className="mr-2"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {game.score}
                          </span>
                          {(game.result === "W" ||
                            game.result === "L" ||
                            game.result === "T") && (
                            <span
                              className="w-5 h-5 rounded-full flex items-center justify-center text-xs"
                              style={{
                                backgroundColor: resultTokens(game.result).fg,
                                color: resultTokens(game.result).bg,
                              }}
                            >
                              {game.result}
                            </span>
                          )}
                          {game.result === "-" && (
                            <span style={{ color: "var(--text-muted)" }}>
                              Upcoming
                            </span>
                          )}
                        </div>
                      </div>
                    ))}
                </div>
              </CardContent>
            </Card>
          </section>

          {/* Add history vs top opponents section */}
          {teamStats?.topOpponents && teamStats.topOpponents.length > 0 && (
            <Card>
              <CardContent className="p-6">
                <h2
                  className="text-xl font-semibold mb-4"
                  style={{ color: "var(--text-primary)" }}
                >
                  History vs Top Opponents
                </h2>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  {teamStats.topOpponents
                    .sort((a, b) => b.record.wins - a.record.wins)
                    .map((opponentData, i) => (
                      <div key={i} className="border rounded-lg p-4">
                        <h3 className="font-medium mb-2">
                          <Link
                            href={`/league/${leagueIdNum}/teams/${opponentData.opponentESPNID}`}
                            className="hover:underline"
                            style={{ color: "var(--action-primary)" }}
                          >
                            {opponentData.opponent}
                          </Link>
                        </h3>
                        <div className="flex items-center justify-between mb-2">
                          <span
                            className="text-sm"
                            style={{ color: "var(--text-muted)" }}
                          >
                            Record:
                          </span>
                          <span
                            className="font-medium"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {opponentData.record.wins}-
                            {opponentData.record.losses}
                            {opponentData.record.ties > 0
                              ? `-${opponentData.record.ties}`
                              : ""}
                          </span>
                        </div>
                        <div className="flex items-center justify-between">
                          <span
                            className="text-sm"
                            style={{ color: "var(--text-muted)" }}
                          >
                            Win %:
                          </span>
                          <span
                            className="font-medium"
                            style={{
                              color:
                                opponentData.winPercentage >= 50
                                  ? "var(--status-success-fg)"
                                  : "var(--status-danger-fg)",
                            }}
                          >
                            {opponentData.winPercentage.toFixed(1)}%
                          </span>
                        </div>
                        <div className="mt-2">
                          <Link
                            href={`/league/${leagueIdNum}/teams/${opponentData.opponentESPNID}`}
                            className="text-xs hover:underline"
                            style={{ color: "var(--action-primary)" }}
                          >
                            View {opponentData.opponent}&apos;s Team Page →
                          </Link>
                        </div>
                      </div>
                    ))}
                </div>
              </CardContent>
            </Card>
          )}

          {/* Year-by-Year Records Table */}
          {yearByYearRecords.length > 0 && (
            <Card>
              <CardContent className="p-6">
                <h2
                  className="text-xl font-semibold mb-4"
                  style={{ color: "var(--text-primary)" }}
                >
                  Year-by-Year Record
                </h2>
                <DataTable
                  columns={yearRecordColumns}
                  rows={yearByYearRecords}
                  rowKey={(yearRecord) => String(yearRecord.year)}
                />
              </CardContent>
            </Card>
          )}
        </TabsContent>

        {/* Players Tab */}
        <TabsContent value="players" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2
                className="text-xl font-semibold mb-4"
                style={{ color: "var(--text-primary)" }}
              >
                Team Roster
              </h2>
              <DataTable
                columns={rosterColumns}
                rows={team.players}
                rowKey={(player) => String(player.id)}
              />
            </CardContent>
          </Card>
        </TabsContent>

        {/* Schedule Tab */}
        <TabsContent value="schedule" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2
                className="text-xl font-semibold mb-4"
                style={{ color: "var(--text-primary)" }}
              >
                Team Schedule
              </h2>

              {/* Add filters */}
              <div className="flex flex-col md:flex-row gap-4 mb-6">
                <div className="w-full md:w-auto">
                  <label
                    htmlFor="year-filter"
                    className="block text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Filter by Year
                  </label>
                  <Select
                    value={yearFilter}
                    onValueChange={(value) => setYearFilter(value)}
                  >
                    <SelectTrigger
                      id="year-filter"
                      aria-label="Filter by Year"
                      className="w-full md:w-auto"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Years</SelectItem>
                      {Array.from(
                        new Set(team.schedule.map((game) => game.year))
                      )
                        .sort((a, b) => b - a) // Sort years in descending order
                        .map((year) => (
                          <SelectItem key={`year-${year}`} value={String(year)}>
                            {year}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="w-full md:w-auto">
                  <label
                    htmlFor="opponent-filter"
                    className="block text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Filter by Opponent
                  </label>
                  <Select
                    value={opponentFilter}
                    onValueChange={(value) => setOpponentFilter(value)}
                  >
                    <SelectTrigger
                      id="opponent-filter"
                      aria-label="Filter by Opponent"
                      className="w-full md:w-auto"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Opponents</SelectItem>
                      {Array.from(
                        new Set(team.schedule.map((game) => game.opponent))
                      )
                        .sort((a, b) => a.localeCompare(b)) // Sort opponents alphabetically
                        .map((opponent) => (
                          <SelectItem
                            key={`opponent-${opponent}`}
                            value={opponent}
                          >
                            {opponent}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </div>

                {/* Reset button - only show when filters are active */}
                {(yearFilter !== "all" || opponentFilter !== "all") && (
                  <div className="w-full md:w-auto flex items-end">
                    <Button
                      variant="outline"
                      onClick={() => {
                        setYearFilter("all");
                        setOpponentFilter("all");
                      }}
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        className="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={2}
                          d="M6 18L18 6M6 6l12 12"
                        />
                      </svg>
                      Reset Filters
                    </Button>
                  </div>
                )}
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {filteredSchedule
                  // Sort by most recent
                  .sort((a, b) => {
                    if (a.year !== b.year) return b.year - a.year; // Most recent year first
                    return b.week - a.week; // Most recent week first
                  })
                  .map((game, i) => {
                    // W / L is a status; anything else (upcoming or tie) keeps
                    // the neutral surface the old gray card used.
                    const cardBorderColor =
                      game.result === "W"
                        ? "var(--status-success-fg)"
                        : game.result === "L"
                        ? "var(--status-danger-fg)"
                        : "var(--border-subtle)";
                    const cardBackgroundColor =
                      game.result === "W"
                        ? "var(--status-success-bg)"
                        : game.result === "L"
                        ? "var(--status-danger-bg)"
                        : "var(--surface-sunken)";

                    const cardContents = (
                      <>
                        <div className="flex justify-between items-center mb-2">
                          <span
                            className="font-medium"
                            style={{ color: "var(--text-primary)" }}
                          >
                            Week {game.week} ({game.year})
                          </span>
                          {game.result !== "-" && (
                            <span
                              className="w-5 h-5 rounded-full flex items-center justify-center text-xs"
                              style={{
                                backgroundColor: resultTokens(game.result).fg,
                                color: resultTokens(game.result).bg,
                              }}
                            >
                              {game.result}
                            </span>
                          )}
                        </div>
                        <div className="mb-2">
                          <span style={{ color: "var(--text-muted)" }}>
                            {game.isHome ? "vs" : "@"}
                          </span>
                          <span
                            className="ml-2 font-medium"
                            style={{ color: "var(--text-primary)" }}
                          >
                            {game.opponent}
                          </span>
                        </div>
                        <div>
                          {game.result !== "-" ? (
                            <span style={{ color: "var(--text-primary)" }}>
                              {game.score}
                            </span>
                          ) : (
                            <span style={{ color: "var(--text-muted)" }}>
                              Upcoming
                            </span>
                          )}
                        </div>
                      </>
                    );

                    return (
                      <div key={i}>
                        {game.matchupId ? (
                          <Link
                            href={`/league/${leagueIdNum}/schedule/${game.matchupId}`}
                            className="block p-4 rounded-lg border cursor-pointer hover:shadow-md transition-shadow duration-200"
                            style={{
                              borderColor: cardBorderColor,
                              backgroundColor: cardBackgroundColor,
                            }}
                          >
                            {cardContents}
                          </Link>
                        ) : (
                          <div
                            className="p-4 rounded-lg border"
                            style={{
                              borderColor: cardBorderColor,
                              backgroundColor: cardBackgroundColor,
                            }}
                          >
                            {cardContents}
                          </div>
                        )}
                      </div>
                    );
                  })}
              </div>
              {/* Show message when no games match filters */}
              {filteredSchedule.length === 0 && (
                <EmptyState title="No games match the selected filters." />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Draft Picks Tab */}
        <TabsContent value="draft" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2
                className="text-xl font-semibold mb-4"
                style={{ color: "var(--text-primary)" }}
              >
                Draft Capital
              </h2>

              {/* Year Filter */}
              <div className="flex flex-col md:flex-row gap-4 mb-6">
                <div className="w-full md:w-auto">
                  <label
                    htmlFor="draft-year-filter"
                    className="block text-sm font-medium mb-1"
                    style={{ color: "var(--text-muted)" }}
                  >
                    Filter by Year
                  </label>
                  <Select
                    value={transactionYearFilter}
                    onValueChange={(value) => setTransactionYearFilter(value)}
                  >
                    <SelectTrigger
                      id="draft-year-filter"
                      aria-label="Filter by Year"
                      className="w-full md:w-auto"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All Years</SelectItem>
                      {Array.from(
                        new Set(
                          team.draftPicks.map(
                            (pick) =>
                              pick.description.match(/\((\d{4})\)/)?.[1] || ""
                          )
                        )
                      )
                        .filter(Boolean)
                        .sort((a, b) => parseInt(b) - parseInt(a))
                        .map((year) => (
                          <SelectItem key={`draft-year-${year}`} value={year}>
                            {year}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <DataTable
                columns={draftPickColumns}
                rows={filteredDraftPicks}
                rowKey={({ index }) => String(index)}
              />

              {/* Show message when no draft picks match the filter */}
              {filteredDraftPicks.length === 0 && (
                <EmptyState title="No draft picks match the selected year filter." />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* Transactions Tab */}
        <TabsContent value="transactions" className="space-y-6">
          <Card>
            <CardContent className="p-6">
              <h2
                className="text-xl font-semibold mb-4"
                style={{ color: "var(--text-primary)" }}
              >
                Team Transactions
              </h2>

              {team && team.transactions && team.transactions.length > 0 ? (
                <div className="space-y-4">
                  {/* Year Filter */}
                  <div className="flex flex-col md:flex-row gap-4 mb-6">
                    <div className="w-full md:w-auto">
                      <label
                        htmlFor="transaction-year-filter"
                        className="block text-sm font-medium mb-1"
                        style={{ color: "var(--text-muted)" }}
                      >
                        Filter by Year
                      </label>
                      <Select
                        value={transactionYearFilter}
                        onValueChange={(value) =>
                          setTransactionYearFilter(value)
                        }
                      >
                        <SelectTrigger
                          id="transaction-year-filter"
                          aria-label="Filter by Year"
                          className="w-full md:w-auto"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="all">All Years</SelectItem>
                          {Array.from(
                            new Set(
                              team.transactions.map(
                                (transaction) => transaction.year
                              )
                            )
                          )
                            .sort((a, b) => b - a)
                            .map((year) => (
                              <SelectItem
                                key={`transaction-year-${year}`}
                                value={String(year)}
                              >
                                {year}
                              </SelectItem>
                            ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  {/* Filter buttons */}
                  <div className="flex flex-wrap gap-2 mb-6 justify-center sm:justify-start">
                    <Button className="rounded-full px-4" onClick={() => {}}>
                      All Types
                    </Button>
                    <Button
                      variant="secondary"
                      className="rounded-full px-4"
                      onClick={() => {}}
                    >
                      Draft
                    </Button>
                    <Button
                      variant="secondary"
                      className="rounded-full px-4"
                      onClick={() => {}}
                    >
                      Trade
                    </Button>
                    <Button
                      variant="secondary"
                      className="rounded-full px-4"
                      onClick={() => {}}
                    >
                      Waiver
                    </Button>
                  </div>

                  {/* Transaction Cards */}
                  <div className="space-y-4">
                    {team.transactions
                      .filter(
                        (transaction) =>
                          transactionYearFilter === "all" ||
                          transaction.year.toString() === transactionYearFilter
                      )
                      .map((transaction) => (
                        <div
                          key={transaction.id}
                          className="border rounded-lg overflow-hidden"
                          style={{
                            borderLeftWidth: 4,
                            borderLeftColor: txTypeColor(transaction.type),
                          }}
                        >
                          <div
                            className="px-4 py-3 flex flex-col sm:flex-row sm:justify-between sm:items-center space-y-2 sm:space-y-0"
                            style={{
                              backgroundColor: "var(--surface-sunken)",
                            }}
                          >
                            <div className="flex flex-col sm:flex-row sm:items-center space-y-2 sm:space-y-0">
                              <Badge
                                variant="outline"
                                className="rounded-full w-fit"
                                style={{
                                  color: txTypeColor(transaction.type),
                                  borderColor: txTypeColor(transaction.type),
                                }}
                              >
                                {transaction.type}
                              </Badge>
                              <span
                                className="sm:ml-3 text-sm"
                                style={{ color: "var(--text-muted)" }}
                              >
                                {transaction.date} - {transaction.year} Week{" "}
                                {transaction.week}
                              </span>
                            </div>
                          </div>

                          <div className="p-4">
                            <p
                              className="mb-4"
                              style={{ color: "var(--text-primary)" }}
                            >
                              {transaction.description}
                            </p>

                            <div className="space-y-4 sm:space-y-0 sm:grid sm:grid-cols-1 lg:grid-cols-2 sm:gap-4">
                              {transaction.playersGained &&
                                transaction.playersGained.length > 0 && (
                                  <div>
                                    <h4
                                      className="text-sm font-medium mb-2"
                                      style={{
                                        color: "var(--status-success-fg)",
                                      }}
                                    >
                                      Players Gained:
                                    </h4>
                                    <ul className="list-disc list-inside space-y-1">
                                      {transaction.playersGained.map(
                                        (player, idx) => (
                                          <li
                                            key={idx}
                                            className="text-sm"
                                            style={{
                                              color: "var(--text-primary)",
                                            }}
                                          >
                                            {typeof player === "object"
                                              ? player.name ||
                                                JSON.stringify(player)
                                              : player}
                                          </li>
                                        )
                                      )}
                                    </ul>
                                  </div>
                                )}

                              {transaction.playersLost &&
                                transaction.playersLost.length > 0 && (
                                  <div>
                                    <h4
                                      className="text-sm font-medium mb-2"
                                      style={{
                                        color: "var(--status-danger-fg)",
                                      }}
                                    >
                                      Players Lost:
                                    </h4>
                                    <ul className="list-disc list-inside space-y-1">
                                      {transaction.playersLost.map(
                                        (player, idx) => (
                                          <li
                                            key={idx}
                                            className="text-sm"
                                            style={{
                                              color: "var(--text-primary)",
                                            }}
                                          >
                                            {typeof player === "object"
                                              ? player.name ||
                                                JSON.stringify(player)
                                              : player}
                                          </li>
                                        )
                                      )}
                                    </ul>
                                  </div>
                                )}
                            </div>
                          </div>
                        </div>
                      ))}
                  </div>

                  {/* Show message when no transactions match the filter */}
                  {team.transactions.filter(
                    (transaction) =>
                      transactionYearFilter === "all" ||
                      transaction.year.toString() === transactionYearFilter
                  ).length === 0 && (
                    <EmptyState title="No transactions match the selected year filter." />
                  )}
                </div>
              ) : (
                <EmptyState title="No transactions found for this team." />
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}

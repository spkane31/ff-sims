import React, { useMemo } from "react";
import { WeeklyExpectedWins } from "../services/expectedWinsService";

interface TeamExpectedWinsChartProps {
  progressionData: WeeklyExpectedWins[];
  isLoading: boolean;
  currentYear: number;
}

interface ChartDataPoint {
  week: number;
  expectedWins: number;
  actualWins: number;
  winLuck: number;
  weeklyWinProbability: number;
  weeklyActualWin: boolean;
  pointDifferential: number;
}

function prepareTeamChartData(
  progressionData: WeeklyExpectedWins[]
): ChartDataPoint[] {
  return progressionData
    .sort((a, b) => a.week - b.week)
    .map((week) => ({
      week: week.week,
      expectedWins: week.expected_wins,
      actualWins: week.actual_wins,
      winLuck: week.win_luck,
      weeklyWinProbability: week.weekly_win_probability,
      weeklyActualWin: week.weekly_actual_win,
      pointDifferential: week.point_differential,
    }));
}

export default function TeamExpectedWinsChart({
  progressionData,
  isLoading,
  currentYear,
}: TeamExpectedWinsChartProps) {
  const chartData = useMemo(
    () => prepareTeamChartData(progressionData),
    [progressionData]
  );
  const maxWeek = useMemo(() => {
    return chartData.reduce((max, week) => Math.max(max, week.week), 0);
  }, [chartData]);

  const maxWins = useMemo(() => {
    return chartData.reduce(
      (max, week) => Math.max(max, week.actualWins, week.expectedWins),
      0
    );
  }, [chartData]);

  const teamName =
    progressionData.length > 0
      ? progressionData[0].team?.name || "Team"
      : "Team";

  if (isLoading) {
    return (
      <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
        <div className="flex items-center justify-center h-40">
          <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-2">Loading expected wins chart...</span>
        </div>
      </div>
    );
  }

  if (chartData.length === 0) {
    return (
      <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
        <h3 className="text-lg font-semibold mb-4">
          {teamName} Expected vs Actual Wins - {currentYear}
        </h3>
        <div className="text-center text-gray-500 dark:text-gray-400">
          No weekly progression data available for this season.
        </div>
      </div>
    );
  }

  const chartWidth = 800;
  const chartHeight = 400;
  const margin = { top: 20, right: 40, bottom: 60, left: 50 };
  const innerWidth = chartWidth - margin.left - margin.right;
  const innerHeight = chartHeight - margin.top - margin.bottom;

  const xScale = (week: number) => ((week - 1) / (maxWeek - 1)) * innerWidth;
  const yScale = (wins: number) => innerHeight - (wins / maxWins) * innerHeight;

  // Generate grid lines
  const xGridLines = [];
  const yGridLines = [];

  for (let week = 1; week <= maxWeek; week++) {
    if (week % 2 === 1 || week === maxWeek) {
      xGridLines.push(
        <line
          key={`x-grid-${week}`}
          x1={xScale(week)}
          y1={0}
          x2={xScale(week)}
          y2={innerHeight}
          stroke="#E5E7EB"
          strokeWidth={0.5}
          opacity={0.3}
        />
      );
    }
  }

  for (let wins = 0; wins <= maxWins; wins += 1) {
    yGridLines.push(
      <line
        key={`y-grid-${wins}`}
        x1={0}
        y1={yScale(wins)}
        x2={innerWidth}
        y2={yScale(wins)}
        stroke="#E5E7EB"
        strokeWidth={0.5}
        opacity={0.3}
      />
    );
  }

  // Generate paths
  const expectedPath = chartData
    .map((point, index) => {
      const x = xScale(point.week);
      const y = yScale(point.expectedWins);
      return index === 0 ? `M ${x} ${y}` : `L ${x} ${y}`;
    })
    .join(" ");

  const actualPath = chartData
    .map((point, index) => {
      const x = xScale(point.week);
      const y = yScale(point.actualWins);
      return index === 0 ? `M ${x} ${y}` : `L ${x} ${y}`;
    })
    .join(" ");

  return (
    <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md">
      <h3 className="text-lg font-semibold mb-4">
        {teamName} Expected vs Actual Wins - {currentYear}
      </h3>
      <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
        Track this team&apos;s actual wins compared to expected wins throughout
        the season
      </p>

      <div className="overflow-x-auto">
        <svg width={chartWidth} height={chartHeight} className="border rounded">
          <g transform={`translate(${margin.left}, ${margin.top})`}>
            {/* Grid lines */}
            {xGridLines}
            {yGridLines}

            {/* Expected wins line (dashed) */}
            <path
              d={expectedPath}
              stroke="#3B82F6"
              strokeWidth={3}
              strokeDasharray="8,4"
              fill="none"
              opacity={0.8}
            />

            {/* Actual wins line (solid) */}
            <path d={actualPath} stroke="#059669" strokeWidth={3} fill="none" />

            {/* Data points with hover information */}
            {chartData.map((point) => (
              <g key={`points-${point.week}`}>
                {/* Expected wins point */}
                <circle
                  cx={xScale(point.week)}
                  cy={yScale(point.expectedWins)}
                  r={4}
                  fill="#3B82F6"
                  opacity={0.8}
                  stroke="white"
                  strokeWidth={2}
                >
                  <title>
                    Week {point.week}: {point.expectedWins.toFixed(2)} expected
                    wins
                  </title>
                </circle>

                {/* Actual wins point */}
                <circle
                  cx={xScale(point.week)}
                  cy={yScale(point.actualWins)}
                  r={4}
                  fill="#059669"
                  stroke="white"
                  strokeWidth={2}
                >
                  <title>
                    Week {point.week}: {point.actualWins} actual wins (Luck:{" "}
                    {point.winLuck > 0 ? "+" : ""}
                    {point.winLuck.toFixed(2)})
                  </title>
                </circle>

                {/* Weekly result indicator */}
                <circle
                  cx={xScale(point.week)}
                  cy={innerHeight + 25}
                  r={6}
                  fill={point.weeklyActualWin ? "#10B981" : "#EF4444"}
                  opacity={0.8}
                >
                  <title>
                    Week {point.week}: {point.weeklyActualWin ? "Win" : "Loss"}
                    (Win Probability:{" "}
                    {(point.weeklyWinProbability * 100).toFixed(1)}%) Point
                    Differential: {point.pointDifferential > 0 ? "+" : ""}
                    {point.pointDifferential.toFixed(2)}
                  </title>
                </circle>
              </g>
            ))}

            {/* Axes */}
            <line
              x1={0}
              y1={innerHeight}
              x2={innerWidth}
              y2={innerHeight}
              stroke="#374151"
              strokeWidth={2}
            />
            <line
              x1={0}
              y1={0}
              x2={0}
              y2={innerHeight}
              stroke="#374151"
              strokeWidth={2}
            />

            {/* X-axis labels */}
            {chartData
              .filter(
                (_, index) => index % 2 === 0 || index === chartData.length - 1
              )
              .map((point) => (
                <text
                  key={`x-label-${point.week}`}
                  x={xScale(point.week)}
                  y={innerHeight + 15}
                  textAnchor="middle"
                  fontSize="12"
                  fill="#6B7280"
                >
                  {point.week}
                </text>
              ))}

            {/* Y-axis labels */}
            {Array.from({ length: maxWins + 1 }, (_, i) => i).map((wins) => (
              <text
                key={`y-label-${wins}`}
                x={-10}
                y={yScale(wins) + 4}
                textAnchor="end"
                fontSize="12"
                fill="#6B7280"
              >
                {wins}
              </text>
            ))}

            {/* Axis titles */}
            <text
              x={innerWidth / 2}
              y={innerHeight + 50}
              textAnchor="middle"
              fontSize="14"
              fill="#374151"
              fontWeight="500"
            >
              Week
            </text>

            <text
              x={-35}
              y={innerHeight / 2}
              textAnchor="middle"
              fontSize="14"
              fill="#374151"
              fontWeight="500"
              transform={`rotate(-90, -35, ${innerHeight / 2})`}
            >
              Cumulative Wins
            </text>
          </g>

          {/* Legend */}
          <g transform={`translate(${chartWidth - 180}, ${margin.top + 10})`}>
            <rect
              x={0}
              y={0}
              width={170}
              height={100}
              fill="#F9FAFB"
              stroke="#E5E7EB"
              strokeWidth={1}
              rx={4}
            />
            <text x={10} y={20} fontSize="12" fontWeight="500" fill="#374151">
              Legend
            </text>

            {/* Expected wins legend */}
            <line
              x1={10}
              y1={35}
              x2={30}
              y2={35}
              stroke="#3B82F6"
              strokeWidth={3}
              strokeDasharray="4,2"
            />
            <text x={35} y={39} fontSize="11" fill="#3B82F6">
              Expected Wins
            </text>

            {/* Actual wins legend */}
            <line
              x1={10}
              y1={50}
              x2={30}
              y2={50}
              stroke="#059669"
              strokeWidth={3}
            />
            <text x={35} y={54} fontSize="11" fill="#059669">
              Actual Wins
            </text>

            {/* Weekly results legend */}
            <circle cx={20} cy={70} r={4} fill="#10B981" />
            <text x={30} y={74} fontSize="11" fill="#374151">
              Win
            </text>

            <circle cx={80} cy={70} r={4} fill="#EF4444" />
            <text x={90} y={74} fontSize="11" fill="#374151">
              Loss
            </text>

            <text x={10} y={90} fontSize="10" fill="#6B7280">
              Hover for details
            </text>
          </g>
        </svg>
      </div>

      {/* Weekly Results Summary */}
      <div className="mt-6 grid grid-cols-2 md:grid-cols-4 gap-4 text-center">
        <div>
          <div className="text-lg font-bold text-green-600 dark:text-green-400">
            {chartData.filter((d) => d.weeklyActualWin).length}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400">Wins</div>
        </div>

        <div>
          <div className="text-lg font-bold text-red-600 dark:text-red-400">
            {chartData.filter((d) => !d.weeklyActualWin).length}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400">Losses</div>
        </div>

        <div>
          <div className="text-lg font-bold text-blue-600 dark:text-blue-400">
            {(
              (chartData.reduce((sum, d) => sum + d.weeklyWinProbability, 0) /
                chartData.length) *
              100
            ).toFixed(1)}
            %
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400">
            Avg Win Prob
          </div>
        </div>

        <div>
          <div
            className={`text-lg font-bold ${
              chartData[chartData.length - 1]?.winLuck > 0
                ? "text-green-600 dark:text-green-400"
                : "text-red-600 dark:text-red-400"
            }`}
          >
            {chartData[chartData.length - 1]?.winLuck > 0 ? "+" : ""}
            {chartData[chartData.length - 1]?.winLuck.toFixed(1)}
          </div>
          <div className="text-sm text-gray-600 dark:text-gray-400">
            Total Luck
          </div>
        </div>
      </div>
    </div>
  );
}

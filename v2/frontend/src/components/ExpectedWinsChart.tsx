import React, { useMemo } from 'react';
import { WeeklyExpectedWins } from '../services/expectedWinsService';

interface ExpectedWinsChartProps {
  weeklyData: WeeklyExpectedWins[];
  isLoading: boolean;
  currentYear: number;
}

interface ChartDataPoint {
  week: number;
  expectedWins: number;
  actualWins: number;
  winLuck: number;
}

function prepareChartData(weeklyData: WeeklyExpectedWins[]): Map<number, ChartDataPoint[]> {
  const teamData = new Map<number, ChartDataPoint[]>();

  weeklyData.forEach((week) => {
    if (!teamData.has(week.team_id)) {
      teamData.set(week.team_id, []);
    }

    teamData.get(week.team_id)!.push({
      week: week.week,
      expectedWins: week.expected_wins,
      actualWins: week.actual_wins,
      winLuck: week.win_luck,
    });
  });

  // Sort each team's data by week
  teamData.forEach((data) => {
    data.sort((a, b) => a.week - b.week);
  });

  return teamData;
}

function generateTeamColor(teamId: number): string {
  const colors = [
    '#3B82F6', '#EF4444', '#10B981', '#F59E0B', '#8B5CF6',
    '#F97316', '#06B6D4', '#84CC16', '#EC4899', '#6366F1',
    '#14B8A6', '#F43F5E', '#8B5A2B', '#6B7280', '#7C3AED'
  ];
  return colors[teamId % colors.length];
}

export default function ExpectedWinsChart({ weeklyData, isLoading, currentYear }: ExpectedWinsChartProps) {
  const chartData = useMemo(() => prepareChartData(weeklyData), [weeklyData]);
  const maxWeek = useMemo(() => {
    return weeklyData.reduce((max, week) => Math.max(max, week.week), 0);
  }, [weeklyData]);

  const maxWins = useMemo(() => {
    return weeklyData.reduce((max, week) => Math.max(max, week.actual_wins, week.expected_wins), 0);
  }, [weeklyData]);

  if (isLoading) {
    return (
      <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md mb-8">
        <div className="flex items-center justify-center h-40">
          <div className="w-6 h-6 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
          <span className="ml-2">Loading expected wins chart...</span>
        </div>
      </div>
    );
  }

  if (chartData.size === 0 || maxWeek === 0) {
    return (
      <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md mb-8">
        <h3 className="text-lg font-semibold mb-4">Expected vs Actual Wins Progression - {currentYear}</h3>
        <div className="text-center text-gray-500 dark:text-gray-400">
          No weekly progression data available for this season.
        </div>
      </div>
    );
  }

  const chartWidth = 800;
  const chartHeight = 400;
  const margin = { top: 20, right: 150, bottom: 40, left: 40 };
  const innerWidth = chartWidth - margin.left - margin.right;
  const innerHeight = chartHeight - margin.top - margin.bottom;

  const xScale = (week: number) => (week / maxWeek) * innerWidth;
  const yScale = (wins: number) => innerHeight - (wins / maxWins) * innerHeight;

  // Generate grid lines
  const xGridLines = [];
  const yGridLines = [];
  
  for (let week = 1; week <= maxWeek; week++) {
    if (week % 2 === 0 || week === 1 || week === maxWeek) {
      xGridLines.push(
        <line
          key={`x-grid-${week}`}
          x1={xScale(week)}
          y1={0}
          x2={xScale(week)}
          y2={innerHeight}
          stroke="#E5E7EB"
          strokeWidth={0.5}
          opacity={0.5}
        />
      );
    }
  }

  for (let wins = 0; wins <= maxWins; wins += 2) {
    yGridLines.push(
      <line
        key={`y-grid-${wins}`}
        x1={0}
        y1={yScale(wins)}
        x2={innerWidth}
        y2={yScale(wins)}
        stroke="#E5E7EB"
        strokeWidth={0.5}
        opacity={0.5}
      />
    );
  }

  return (
    <div className="bg-white dark:bg-gray-700 p-6 rounded-lg shadow-md mb-8">
      <h3 className="text-lg font-semibold mb-4">Expected vs Actual Wins Progression - {currentYear}</h3>
      <p className="text-sm text-gray-600 dark:text-gray-400 mb-4">
        Track how each team's actual wins compare to their expected wins throughout the season
      </p>
      
      <div className="overflow-x-auto">
        <svg width={chartWidth} height={chartHeight} className="border rounded">
          <g transform={`translate(${margin.left}, ${margin.top})`}>
            {/* Grid lines */}
            {xGridLines}
            {yGridLines}

            {/* Data lines for each team */}
            {Array.from(chartData.entries()).map(([teamId, data]) => {
              const color = generateTeamColor(teamId);
              const teamInfo = weeklyData.find(w => w.team_id === teamId)?.team;
              
              if (!teamInfo || data.length === 0) return null;

              // Expected wins line (dashed)
              const expectedPath = data.map((point, index) => {
                const x = xScale(point.week);
                const y = yScale(point.expectedWins);
                return index === 0 ? `M ${x} ${y}` : `L ${x} ${y}`;
              }).join(' ');

              // Actual wins line (solid)
              const actualPath = data.map((point, index) => {
                const x = xScale(point.week);
                const y = yScale(point.actualWins);
                return index === 0 ? `M ${x} ${y}` : `L ${x} ${y}`;
              }).join(' ');

              return (
                <g key={teamId}>
                  {/* Expected wins line (dashed) */}
                  <path
                    d={expectedPath}
                    stroke={color}
                    strokeWidth={2}
                    strokeDasharray="5,5"
                    fill="none"
                    opacity={0.7}
                  />
                  
                  {/* Actual wins line (solid) */}
                  <path
                    d={actualPath}
                    stroke={color}
                    strokeWidth={2}
                    fill="none"
                  />

                  {/* Data points */}
                  {data.map((point) => (
                    <g key={`${teamId}-${point.week}`}>
                      {/* Expected wins point */}
                      <circle
                        cx={xScale(point.week)}
                        cy={yScale(point.expectedWins)}
                        r={3}
                        fill={color}
                        opacity={0.7}
                      />
                      {/* Actual wins point */}
                      <circle
                        cx={xScale(point.week)}
                        cy={yScale(point.actualWins)}
                        r={3}
                        fill={color}
                      />
                    </g>
                  ))}
                </g>
              );
            })}

            {/* Axes */}
            <line x1={0} y1={innerHeight} x2={innerWidth} y2={innerHeight} stroke="#374151" strokeWidth={1} />
            <line x1={0} y1={0} x2={0} y2={innerHeight} stroke="#374151" strokeWidth={1} />

            {/* X-axis labels */}
            {Array.from({ length: maxWeek }, (_, i) => i + 1)
              .filter(week => week % 2 === 0 || week === 1 || week === maxWeek)
              .map((week) => (
                <text
                  key={`x-label-${week}`}
                  x={xScale(week)}
                  y={innerHeight + 15}
                  textAnchor="middle"
                  fontSize="12"
                  fill="#6B7280"
                >
                  {week}
                </text>
              ))}

            {/* Y-axis labels */}
            {Array.from({ length: Math.ceil(maxWins / 2) + 1 }, (_, i) => i * 2).map((wins) => (
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
              y={innerHeight + 35}
              textAnchor="middle"
              fontSize="14"
              fill="#374151"
              fontWeight="500"
            >
              Week
            </text>
            
            <text
              x={-25}
              y={innerHeight / 2}
              textAnchor="middle"
              fontSize="14"
              fill="#374151"
              fontWeight="500"
              transform={`rotate(-90, -25, ${innerHeight / 2})`}
            >
              Wins
            </text>
          </g>

          {/* Legend */}
          <g transform={`translate(${chartWidth - 140}, ${margin.top + 10})`}>
            <rect x={0} y={0} width={130} height={60} fill="#F9FAFB" stroke="#E5E7EB" strokeWidth={1} rx={4} />
            <text x={10} y={20} fontSize="12" fontWeight="500" fill="#374151">Legend</text>
            
            <line x1={10} y1={35} x2={30} y2={35} stroke="#6B7280" strokeWidth={2} />
            <text x={35} y={39} fontSize="11" fill="#6B7280">Actual Wins</text>
            
            <line x1={10} y1={50} x2={30} y2={50} stroke="#6B7280" strokeWidth={2} strokeDasharray="3,3" />
            <text x={35} y={54} fontSize="11" fill="#6B7280">Expected Wins</text>
          </g>
        </svg>
      </div>
    </div>
  );
}
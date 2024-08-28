import * as React from "react";
import schedule from "../../data/schedule.json";
import team_avgs from "../../data/team_avgs.json";
import { DataGrid } from "@mui/x-data-grid";
import { Box, Paper } from "@mui/material";

export default function Home() {
  return (
    <>
      <h1>Simulations</h1>
      <TeamData />
      <Schedule />
    </>
  );
}

const Schedule = () => {
  return (
    <ul>
      {schedule.map((week, weekIdx) => (
        <li key={weekIdx}>
          <h2>Week {weekIdx + 1}</h2>
          <ul>
            {week.map((game, gameIdx) => (
              <TeamMatchup game={game} key={gameIdx} />
            ))}
          </ul>
        </li>
      ))}
    </ul>
  );
};

const TeamMatchup = ({ game, numSimulations = 100 }) => {
  // const [homeWins, setHomeWins] = React.useState(0);
  // const [awayWins, setAwayWins] = React.useState(0);

  const { average: home_average, std_dev: home_std_dev } =
    team_avgs[game.home_team_owner];
  const { average: away_average, std_dev: away_std_dev } =
    team_avgs[game.away_team_owner];

  const runMonteCarloSimulation = (
    home_average,
    home_std_dev,
    away_average,
    away_std_dev,
    numSimulations
  ) => {
    let homeWins = 0;
    let awayWins = 0;

    for (let i = 0; i < numSimulations; i++) {
      const homeScore = Math.random() * home_std_dev + home_average;
      const awayScore = Math.random() * away_std_dev + away_average;

      if (homeScore > awayScore) {
        homeWins++;
        // setHomeWins(homeWins + 1);
      } else {
        awayWins++;
        // setAwayWins(awayWins + 1);
      }
    }
    return { homeWins, awayWins };
  };

  const { homeWins, awayWins } = runMonteCarloSimulation(
    home_average,
    home_std_dev,
    away_average,
    away_std_dev,
    numSimulations
  );
  const awayWinPercentage = awayWins / (homeWins + awayWins);
  const homeWinPercentage = homeWins / (homeWins + awayWins);

  // TODO seankane: this function is AI created but it looks wonky, add this later
  const convertToBettingOdds = (percentage) => {
    const decimalOdds = percentage; //100 / percentage;
    const americanOdds =
      decimalOdds > 2
        ? Math.round((decimalOdds - 1) * 100)
        : -Math.round(100 / (decimalOdds - 1));
    const adjustedOdds = Math.round(americanOdds * 1.05);
    return adjustedOdds > 0 ? `+${adjustedOdds}` : adjustedOdds.toString();
  };

  // const homeBettingOdds = convertToBettingOdds(homeWinPercentage);
  // const awayBettingOdds = convertToBettingOdds(awayWinPercentage);

  return (
    <li>
      {game.away_team_owner} ({(100 * awayWinPercentage).toFixed(3)} %) @{" "}
      {game.home_team_owner} ({(100 * homeWinPercentage).toFixed(3)} %)
    </li>
  );
};

const TeamData = () => {
  const columns = [
    { field: "teamName", headerName: "Team Name", flex: 1 },
    { field: "average", headerName: "Average", flex: 1 },
    { field: "std_dev", headerName: "Standard Deviation", flex: 1 },
  ];

  const rows = Object.entries(team_avgs).map(
    ([teamName, { average, std_dev }]) => ({
      id: teamName,
      teamName,
      average: average.toFixed(2),
      std_dev: std_dev.toFixed(2),
    })
  );

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 500,
          minHeight: 400,
        }}
      >
        <DataGrid
          columns={columns}
          rows={rows}
          autosizeOnMount
          autoHeight
          hideFooter
        />
      </Paper>
    </Box>
  );
};

import * as React from "react";
import schedule from "../../data/schedule.json";
import team_avgs from "../../data/team_avgs.json";
import { DataGrid } from "@mui/x-data-grid";
import {
  Box,
  Paper,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Button,
  Select,
  MenuItem,
} from "@mui/material";
import Simulator from "../../simulation/simulator";

export default function Home() {
  const [simulator, setSimulator] = React.useState(null);
  const [teamData, setTeamData] = React.useState(null);
  const [steps, setSteps] = React.useState(1);
  const [totalRunTime, setTotalRunTime] = React.useState(0);

  React.useEffect(() => {
    setSimulator(new Simulator());
  }, []);

  React.useEffect(() => {
    if (simulator !== null) {
      console.log(
        "setting team scoring data, this should only happen one time"
      );
      setTeamData(simulator.getTeamScoringData());
    }
  }, [simulator]);

  const handleChange = (event) => {
    setSteps(event.target.value);
  };

  return simulator === null ? (
    <div>Loading...</div>
  ) : (
    <Box
      sx={{
        padding: "5rem 0",
        flex: 1,
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
        overflowX: "auto",
        paddingLeft: "5%",
        paddingRight: "5%",
      }}
    >
      <Button
        onClick={() => {
          const start = new Date().getTime();
          for (let i = 0; i < steps; i++) {
            simulator.step();
          }
          const end = new Date().getTime();
          setTotalRunTime(totalRunTime + (end - start));
          setTeamData(simulator.getTeamScoringData());
        }}
        variant="contained"
      >
        Simulate (n={simulator.simulations})
      </Button>
      {/* Add a drop menu to optionally set step to 1, 5, 10, 25, 50 */}
      <Select
        labelId="demo-simple-select-label"
        id="demo-simple-select"
        value={steps}
        label="Step Size"
        onChange={handleChange}
      >
        <MenuItem value={1}>1</MenuItem>
        <MenuItem value={5}>5</MenuItem>
        <MenuItem value={10}>10</MenuItem>
        <MenuItem value={25}>25</MenuItem>
        <MenuItem value={50}>50</MenuItem>
        <MenuItem value={100}>100</MenuItem>
        <MenuItem value={500}>500</MenuItem>
      </Select>
      <Paper>Total Run Time: {totalRunTime}ms</Paper>
      <TeamData teamData={teamData} />
      <Box sx={{ marginTop: "25px" }} />
      <Schedule />
    </Box>
  );
}

const Schedule = ({ simulator }) => {
  return (
    <>
      <Typography variant="h5" sx={{ textAlign: "center" }}>
        Season Prediction
      </Typography>

      <TableContainer component={Paper}>
        <Table stickyHeader>
          <TableHead>
            <TableRow>
              <TableCell>Home Team</TableCell>
              <TableCell>Home Win Percentage</TableCell>
              <TableCell>Away Team</TableCell>
              <TableCell>Away Win Percentage</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {schedule.map((week, weekIdx) => (
              <React.Fragment key={weekIdx}>
                <TableRow>
                  <TableCell colSpan={4} align="center">
                    <Typography variant="h6">Week {weekIdx + 1}</Typography>
                  </TableCell>
                </TableRow>
                {week.map((game, gameIdx) => (
                  <TeamMatchup game={game} key={gameIdx} />
                ))}
              </React.Fragment>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};

const TeamMatchup = ({ game, numSimulations = 500 }) => {
  const { average: home_average, std_dev: home_std_dev } =
    team_avgs[game.home_team_owner];
  const { average: away_average, std_dev: away_std_dev } =
    team_avgs[game.away_team_owner];
  const { average: league_average, std_dev: league_std_dev } =
    team_avgs["League"];

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
      const jitterPercentageHome = Math.random() * 0.1 + 0.05;
      const jitterPercentageAway = Math.random() * 0.1 + 0.05;

      const leagueJitterHome = Math.random() * league_std_dev + home_average;
      const leagueJitterAway = Math.random() * league_std_dev + away_average;

      const homeScore =
        (1 - jitterPercentageHome) * Math.random() * home_std_dev +
        home_average +
        jitterPercentageHome * leagueJitterHome;
      const awayScore =
        (1 - jitterPercentageAway) * Math.random() * away_std_dev +
        away_average +
        jitterPercentageAway * leagueJitterAway;

      if (homeScore > awayScore) {
        homeWins++;
      } else {
        awayWins++;
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

  return (
    <TableRow>
      <TableCell>{game.home_team_owner}</TableCell>
      <TableCell>{(100 * homeWinPercentage).toFixed(3)}%</TableCell>
      <TableCell>{game.away_team_owner}</TableCell>
      <TableCell>{(100 * awayWinPercentage).toFixed(3)}%</TableCell>
    </TableRow>
  );
};

const TeamData = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "Team Name", flex: 1 },
    { field: "average", headerName: "Average", flex: 1 },
    {
      field: "std_dev",
      headerName: "Standard Deviation",
      flex: 1,
      type: "number",
    },
    {
      field: "projected_wins",
      headerName: "Projected Wins",
      flex: 1,
      type: "number",
    },
    {
      field: "projected_losses",
      headerName: "Projected Losses",
      flex: 1,
      type: "number",
    },
    {
      field: "playoff_odds",
      headerName: "Playoff Odds",
      flex: 1,
      type: "number",
    },
    {
      field: "last_place_odds",
      headerName: "Last Place Odds",
      flex: 1,
      type: "number",
    },
  ];

  const rows2 = Object.entries(teamData).map(([teamName, teamResults]) => {
    return {
      id: teamResults.id,
      teamName: teamResults.teamName,
      average: teamResults.average.toFixed(2),
      std_dev: teamResults.std_dev.toFixed(2),
      projected_wins: teamResults.wins.toFixed(2),
      projected_losses: teamResults.losses.toFixed(2),
      playoff_odds: (100 * teamResults.playoff_odds).toFixed(2),
      last_place_odds: (100 * teamResults.last_place_odds).toFixed(2),
    };
  });

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 750,
          minHeight: 400,
        }}
      >
        <Typography variant="h5" sx={{ textAlign: "center" }}>
          Team Scoring Data
        </Typography>
        <DataGrid
          columns={columns}
          rows={rows2}
          autosizeOnMount
          autoHeight
          hideFooter
        />
      </Paper>
    </Box>
  );
};

import React from "react";
import schedule from "../../data/schedule.json";
import {
  Paper,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from "@mui/material";

const Schedule = () => {
  return (
    <div>
      <h1>I'll get here eventually</h1>
    </div>
  );
};

export default Schedule;

const sched = ({ simulator }) => {
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

import React from "react";
import {
  Paper,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Box,
} from "@mui/material";
import Simulator from "../../simulation/simulator";
import { normalDistribution } from "../../utils/math";
import teamAvgs from "../../data/team_avgs.json";

const Schedule = () => {
  const [simulator, setSimulator] = React.useState(null);
  const [teamStats, setTeamStats] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);

  React.useEffect(() => {
    if (teamStats !== null && schedule !== null) {
      setSimulator(new Simulator(teamStats, schedule));
    }
  }, [teamStats, schedule]);

  React.useEffect(() => {
    fetch("/api/teams")
      .then((res) => res.json())
      .then((data) => {
        setTeamStats(data);
      });
  }, []);

  React.useEffect(() => {
    fetch("/api/schedule")
      .then((res) => res.json())
      .then((data) => {
        setSchedule(data);
      });
  }, []);

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
      <Box sx={{ marginTop: "15px" }} />
      <Typography variant="h4" sx={{ textAlign: "center" }}>
        Schedule
      </Typography>
      <Box sx={{ marginTop: "15px" }} />
      <ScheduleTable
        schedule={schedule}
        simulator={simulator}
        teamAvgs={teamAvgs}
      />
      <Box sx={{ marginTop: "25px" }} />
    </Box>
  );
};

const ScheduleTable = ({ schedule, simulator }) => {
  if (simulator === null || schedule === null) {
    return <div>Loading...</div>;
  }

  return (
    <TableContainer component={Paper}>
      <Table stickyHeader size="small">
        <TableHead>
          <TableRow rowHeight={25}>
            <TableCell>Home Team</TableCell>
            <TableCell>Win Percentage</TableCell>
            <TableCell>Projected Score</TableCell>
            <TableCell>Score</TableCell>
            <TableCell align="right">Score</TableCell>
            <TableCell align="right">Win Percentage</TableCell>
            <TableCell align="right">Projected Team</TableCell>
            <TableCell align="right">Awat Team</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {schedule.map((week, weekIdx) => (
            <React.Fragment key={weekIdx}>
              <TableRow rowHeight={25}>
                <TableCell colSpan={8} align="center">
                  <Typography variant="h6">Week {weekIdx + 1}</Typography>
                </TableCell>
              </TableRow>
              {week.map((game, gameIdx) => (
                <TeamMatchup game={game} teamAvgs={teamAvgs} key={gameIdx} />
              ))}
            </React.Fragment>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

const TeamMatchup = ({ game, teamAvgs, numSimulations = 1000 }) => {
  if (game === undefined || teamAvgs === undefined) {
    return <></>;
  }

  const { average: home_average, std_dev: home_std_dev } =
    teamAvgs[game.home_team_owner];
  const { average: away_average, std_dev: away_std_dev } =
    teamAvgs[game.away_team_owner];
  const { average: league_average, std_dev: league_std_dev } =
    teamAvgs["League"];

  const runMonteCarloSimulation = (
    home_average,
    home_std_dev,
    away_average,
    away_std_dev,
    league_avg,
    league_std_dev,
    numSimulations
  ) => {
    let homeWins = 0;
    let awayWins = 0;
    let homePoints = 0;
    let awayPoints = 0;

    for (let i = 0; i < numSimulations; i++) {
      const jitterPercentageHome = Math.random() * 0.2 + 0.05;
      const jitterPercentageAway = Math.random() * 0.2 + 0.05;

      const leagueJitterHome =
        jitterPercentageHome * normalDistribution(league_avg, league_std_dev);
      const leagueJitterAway =
        jitterPercentageHome * normalDistribution(league_avg, league_std_dev);

      const homeScore =
        (1 - jitterPercentageHome) *
          normalDistribution(home_average, home_std_dev) +
        jitterPercentageHome * leagueJitterHome;
      const awayScore =
        (1 - jitterPercentageAway) *
          normalDistribution(away_average, away_std_dev) +
        jitterPercentageAway * leagueJitterAway;

      if (homeScore > awayScore) {
        homeWins++;
      } else {
        awayWins++;
      }
      homePoints += homeScore;
      awayPoints += awayScore;
    }
    homePoints /= numSimulations;
    awayPoints /= numSimulations;
    return { homeWins, awayWins, homePoints, awayPoints };
  };

  const { homeWins, awayWins, homePoints, awayPoints } =
    runMonteCarloSimulation(
      home_average,
      home_std_dev,
      away_average,
      away_std_dev,
      league_average,
      league_std_dev,
      numSimulations
    );
  const awayWinPercentage = awayWins / (homeWins + awayWins);
  const homeWinPercentage = homeWins / (homeWins + awayWins);

  return (
    <TableRow>
      <TableCell>{game.home_team_owner}</TableCell>
      <TableCell>{(100 * homeWinPercentage).toFixed(2)}%</TableCell>
      <TableCell>{homePoints.toFixed(2)}</TableCell>
      <TableCell></TableCell>
      <TableCell></TableCell>
      <TableCell align="right">{awayPoints.toFixed(2)}</TableCell>
      <TableCell align="right">
        {(100 * awayWinPercentage).toFixed(2)}%
      </TableCell>
      <TableCell align="right">{game.away_team_owner}</TableCell>
    </TableRow>
  );
};

export default Schedule;

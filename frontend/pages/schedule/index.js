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
      .then((res) => {
        if (res.status === 304) {
          alert("304 Not Modified");
        }
        return res.json();
      })
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
    <>
      <Box sx={{ marginTop: "25px" }} />
      <Typography variant="h4">Schedule</Typography>
      <Box sx={{ marginTop: "25px" }} />
      <ScheduleTable
        schedule={schedule}
        simulator={simulator}
        teamAvgs={teamStats}
      />
      <Box sx={{ marginTop: "25px" }} />
    </>
  );
};

const ScheduleTable = ({ schedule, simulator, teamAvgs }) => {
  if (simulator === null || schedule === null) {
    return <div>Loading...</div>;
  }

  return (
    <TableContainer component={Paper}>
      <Table stickyHeader size="small">
        <TableHead>
          <TableRow sx={{ height: "25px" }}>
            <TableCell>Home Team</TableCell>
            <TableCell>Win Percentage</TableCell>
            <TableCell>Projected Score</TableCell>
            <TableCell>Score</TableCell>
            <TableCell align="right">Score</TableCell>
            <TableCell align="right">Projected Score</TableCell>
            <TableCell align="right">Win Percentage</TableCell>
            <TableCell align="right">Away Team</TableCell>
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

const TeamMatchup = ({ game, teamAvgs, numSimulations = 5000 }) => {
  if (game === undefined || teamAvgs === undefined) {
    console.log(`game ${game} or teamAvgs ${teamAvgs} is undefined`);
    return <></>;
  }

  // fine obj with owner == game.home_team_owner in teamAvgs list
  const homeTeam = teamAvgs.find((team) => team.owner === game.home_team_owner);
  const awayTeam = teamAvgs.find((team) => team.owner === game.away_team_owner);
  const league = teamAvgs.find((team) => team.owner === "League");

  if (
    homeTeam === undefined ||
    awayTeam === undefined ||
    league === undefined
  ) {
    console.log(
      `homeTeam ${homeTeam} or awayTeam ${awayTeam} or league ${league} is undefined`
    );
    return <></>;
  }

  const { averageScore: home_average, stddevScore: home_std_dev } = homeTeam;
  const { averageScore: away_average, stddevScore: away_std_dev } = awayTeam;
  const { averageScore: league_average, stddevScore: league_std_dev } = league;

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

      const homeScore =
        (1 - jitterPercentageHome) *
          normalDistribution(home_average, home_std_dev) +
        jitterPercentageHome * normalDistribution(league_avg, league_std_dev);
      const awayScore =
        (1 - jitterPercentageAway) *
          normalDistribution(away_average, away_std_dev) +
        jitterPercentageHome * normalDistribution(league_avg, league_std_dev);

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
    <TableRow style={{ height: "20px" }}>
      <TableCell>{game.home_team_owner}</TableCell>
      <TableCell>{(100 * homeWinPercentage).toFixed(2)}%</TableCell>
      <TableCell>{game.home_team_espn_projected_score.toFixed(2)}</TableCell>
      <TableCell>{homePoints.toFixed(2)}</TableCell>
      <TableCell align="right">{awayPoints.toFixed(2)}</TableCell>
      <TableCell>{game.away_team_espn_projected_score.toFixed(2)}</TableCell>
      <TableCell align="right">
        {(100 * awayWinPercentage).toFixed(2)}%
      </TableCell>
      <TableCell align="right">{game.away_team_owner}</TableCell>
    </TableRow>
  );
};

export default Schedule;

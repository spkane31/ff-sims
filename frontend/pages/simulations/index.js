import React from "react";
import { Box, Paper, Typography, Button } from "@mui/material";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Simulator from "../../simulation/simulator";

const prettyPrintMilliseconds = (ms) => {
  if (ms < 1000) {
    return `${ms}ms`;
  } else if (ms < 60000) {
    return `${(ms / 1000).toFixed(2)}s`;
  } else {
    // get minutes
    const minutes = Math.floor(ms / 60000);
    const seconds = (ms % 60000) / 1000;
    return `${minutes}m ${seconds.toFixed(2)}s`;
  }
};

function convertToScientificNotation(num) {
  if (num < 0.0001) {
    return num.toExponential(5);
  }
  return num.toFixed(5);
}

export default function Simulations() {
  const [simulator, setSimulator] = React.useState(null);
  const [teamData, setTeamData] = React.useState(null);
  const [teamStats, setTeamStats] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);
  const [steps, setSteps] = React.useState(10000);
  const [totalRunTime, setTotalRunTime] = React.useState(0);

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

  React.useEffect(() => {
    if (simulator !== null) {
      console.log(
        "setting team scoring data, this should only happen one time"
      );
      setTeamData(simulator.getTeamScoringData());
    }
  }, [simulator]);

  return simulator === null ? (
    <div>Loading...</div>
  ) : (
    <>
      <Box
        sx={{
          paddingTop: "20px",
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
          sx={{ marginRight: "10px" }}
        >
          Simulate ({steps.toLocaleString()} steps)
        </Button>
        <Button
          onClick={() => {
            setSimulator(new Simulator(teamStats, schedule));
            setTotalRunTime(0);
          }}
          variant="contained"
          sx={{ marginLeft: "10px" }}
        >
          Reset
        </Button>
      </Box>
      <Box sx={{ marginTop: "15px" }} />
      <Typography variant="h6" sx={{ textAlign: "center" }}>
        Total Run Time: {prettyPrintMilliseconds(totalRunTime)} (N ={" "}
        {simulator.simulations.toLocaleString()})
      </Typography>
      <Typography variant="h6" sx={{ textAlign: "center" }}>
        Average Run Time:{" "}
        {totalRunTime === 0
          ? "-"
          : (totalRunTime / simulator.simulations).toFixed(3)}{" "}
        ms
      </Typography>
      <Typography variant="h6" sx={{ textAlign: "center" }}>
        ε (error): {convertToScientificNotation(simulator.epsilon)}
      </Typography>
      <Box sx={{ marginTop: "15px" }} />
      <TeamData teamData={teamData} />
      <Box sx={{ marginTop: "25px" }} />
      <RegularSeasonPositions teamData={teamData} />
      <Box sx={{ marginTop: "25px" }} />
      <PlayoffPositions teamData={teamData} />
    </>
  );
}

const TeamData = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "", flex: 1, minWidth: 130 },
    { field: "average", headerName: "Average", flex: 1, type: "number" },
    {
      field: "std_dev",
      headerName: "Std. Dev.",
      flex: 1,
      type: "number",
    },
    { field: "wins", headerName: "Wins", flex: 1, type: "number" },
    { field: "losses", headerName: "Losses", flex: 1, type: "number" },
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

  const rows = Object.entries(teamData)
    .map(([_teamName, teamResults]) => {
      return {
        id: teamResults.id,
        owner: teamResults.teamName,
        average: teamResults.average.toFixed(2),
        stdDev: teamResults.std_dev.toFixed(2),
        wins: teamResults.wins.toFixed(2),
        losses: teamResults.losses.toFixed(2),
        playoffOdds: (100 * teamResults.playoff_odds).toFixed(2),
        lastPlaceOdds: (100 * teamResults.last_place_odds).toFixed(2),
      };
    })
    .sort((a, b) => b.projected_wins - a.projected_wins);

  return (
    <>
      <Box
        sx={{
          maxWidth: "100%",
          // overflowX: "auto",
        }}
      >
        <Paper
          sx={{
            // minWidth: 750,
            // minHeight: 400,
            paddingTop: "10px",
          }}
        >
          <Typography
            variant="h5"
            sx={{ textAlign: "center", marginBottom: "10px" }}
          >
            Team Scoring Data
          </Typography>
          <TableContainer component={Paper}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  {columns.map((column) => (
                    <TableCell key={column.field} align="right">
                      {column.headerName}
                    </TableCell>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {rows.map((row) => (
                  <TableRow
                    key={row.id}
                    sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
                  >
                    <TableCell component="th" scope="row">
                      {row.owner}
                    </TableCell>
                    <TableCell align="right">{row.average}</TableCell>
                    <TableCell align="right">{row.stdDev}</TableCell>
                    <TableCell align="right">{row.wins}</TableCell>
                    <TableCell align="right">{row.losses}</TableCell>
                    <TableCell align="right">{row.playoffOdds}</TableCell>
                    <TableCell align="right">{row.lastPlaceOdds}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Paper>
      </Box>
    </>
  );
};

const RegularSeasonPositions = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "", flex: 1, minWidth: 130 },
    { field: "firstPlace", headerName: "1st", flex: 1, type: "number" },
    {
      field: "secondPlace",
      headerName: "2nd",
      flex: 1,
      type: "number",
    },
    { field: "thirdPlace", headerName: "3rd", flex: 1, type: "number" },
    {
      field: "fourthPlace",
      headerName: "4th",
      flex: 1,
      type: "number",
    },
    { field: "fifthPlace", headerName: "5th", flex: 1, type: "number" },
    { field: "sixthPlace", headerName: "6th", flex: 1, type: "number" },
    {
      field: "seventhPlace",
      headerName: "7th",
      flex: 1,
      type: "number",
    },
    {
      field: "eighthPlace",
      headerName: "8th",
      flex: 1,
      type: "number",
    },
    { field: "ninthPlace", headerName: "9th", flex: 1, type: "number" },
    { field: "tenthPlace", headerName: "10th", flex: 1, type: "number" },
  ];

  const rows = Object.entries(teamData)
    .map(([_teamName, teamResults]) => {
      return {
        id: teamResults.id,
        owner: teamResults.teamName,
        firstPlace: teamResults.regular_season_result[0],
        secondPlace: teamResults.regular_season_result[1],
        thirdPlace: teamResults.regular_season_result[2],
        fourthPlace: teamResults.regular_season_result[3],
        fifthPlace: teamResults.regular_season_result[4],
        sixthPlace: teamResults.regular_season_result[5],
        seventhPlace: teamResults.regular_season_result[6],
        eighthPlace: teamResults.regular_season_result[7],
        ninthPlace: teamResults.regular_season_result[8],
        tenthPlace: teamResults.regular_season_result[9],
      };
    })
    .sort((a, b) => {
      if (a.firstPlace !== b.firstPlace) {
        return b.firstPlace - a.firstPlace;
      }
      if (a.secondPlace !== b.secondPlace) {
        return b.secondPlace - a.secondPlace;
      }
      if (a.thirdPlace !== b.thirdPlace) {
        return b.thirdPlace - a.thirdPlace;
      }
      if (a.fourthPlace !== b.fourthPlace) {
        return b.fourthPlace - a.fourthPlace;
      }
      if (a.fifthPlace !== b.fifthPlace) {
        return b.fifthPlace - a.fifthPlace;
      }
      if (a.sixthPlace !== b.sixthPlace) {
        return b.sixthPlace - a.sixthPlace;
      }
      if (a.seventhPlace !== b.seventhPlace) {
        return b.seventhPlace - a.seventhPlace;
      }
      if (a.eighthPlace !== b.eighthPlace) {
        return b.eighthPlace - a.eighthPlace;
      }
      if (a.ninthPlace !== b.ninthPlace) {
        return b.ninthPlace - a.ninthPlace;
      }
      if (a.tenthPlace !== b.tenthPlace) {
        return b.tenthPlace - a.tenthPlace;
      }
      return -1;
    });

  return (
    <Box
      sx={{
        maxWidth: "100%",
        // overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 750,
          // minHeight: 400,
        }}
      >
        <Typography
          variant="h5"
          sx={{ textAlign: "center", marginBottom: "10px" }}
        >
          Regular Season Projections
        </Typography>
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                {columns.map((column) => (
                  <TableCell key={column.field} align="right">
                    {column.headerName}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((row) => (
                <TableRow
                  key={row.id}
                  sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
                >
                  <TableCell component="th" scope="row">
                    {row.owner}
                  </TableCell>
                  <TableCell align="right">
                    {row.firstPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.secondPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.thirdPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.fourthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.fifthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.sixthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.seventhPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.eighthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.ninthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.tenthPlace.toFixed(3)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Paper>
    </Box>
  );
};

const PlayoffPositions = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "", flex: 1, minWidth: 130 },
    { field: "firstPlace", headerName: "1st", flex: 1, type: "number" },
    {
      field: "secondPlace",
      headerName: "2nd",
      flex: 1,
      type: "number",
    },
    { field: "thirdPlace", headerName: "3rd", flex: 1, type: "number" },
    {
      field: "fourthPlace",
      headerName: "4th",
      flex: 1,
      type: "number",
    },
    { field: "fifthPlace", headerName: "5th", flex: 1, type: "number" },
    { field: "sixthPlace", headerName: "6th", flex: 1, type: "number" },
  ];

  const rows = Object.entries(teamData)
    .map(([_teamName, teamResults]) => {
      return {
        id: teamResults.id,
        owner: teamResults.teamName,
        firstPlace: teamResults.playoff_result[0],
        secondPlace: teamResults.playoff_result[1],
        thirdPlace: teamResults.playoff_result[2],
        fourthPlace: teamResults.playoff_result[3],
        fifthPlace: teamResults.playoff_result[4],
        sixthPlace: teamResults.playoff_result[5],
      };
    })
    .sort((a, b) => {
      if (a.firstPlace !== b.firstPlace) {
        return b.firstPlace - a.firstPlace;
      }
      if (a.secondPlace !== b.secondPlace) {
        return b.secondPlace - a.secondPlace;
      }
      if (a.thirdPlace !== b.thirdPlace) {
        return b.thirdPlace - a.thirdPlace;
      }
      if (a.fourthPlace !== b.fourthPlace) {
        return b.fourthPlace - a.fourthPlace;
      }
      if (a.fifthPlace !== b.fifthPlace) {
        return b.fifthPlace - a.fifthPlace;
      }
      if (a.sixthPlace !== b.sixthPlace) {
        return b.sixthPlace - a.sixthPlace;
      }
      return -1;
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
        <Typography
          variant="h5"
          sx={{ textAlign: "center", marginBottom: "10px" }}
        >
          Playoff Projections
        </Typography>

        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                {columns.map((column) => (
                  <TableCell key={column.field} align="right">
                    {column.headerName}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((row) => (
                <TableRow
                  key={row.id}
                  sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
                >
                  <TableCell component="th" scope="row">
                    {row.owner}
                  </TableCell>
                  <TableCell align="right">
                    {row.firstPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.secondPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.thirdPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.fourthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.fifthPlace.toFixed(3)}
                  </TableCell>
                  <TableCell align="right">
                    {row.sixthPlace.toFixed(3)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Paper>
    </Box>
  );
};

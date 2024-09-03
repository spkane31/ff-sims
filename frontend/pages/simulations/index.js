import React from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Box, Paper, Typography, Button } from "@mui/material";
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

export default function Home() {
  const [simulator, setSimulator] = React.useState(null);
  const [teamData, setTeamData] = React.useState(null);
  const [teamStats, setTeamStats] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);
  const [steps, setSteps] = React.useState(25000);
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
      <Box>
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
          Simulate (25,000)
        </Button>
        <Button
          onClick={() => {
            setSimulator(new Simulator());
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
      <Box sx={{ marginTop: "15px" }} />
      <TeamData teamData={teamData} />
      <Box sx={{ marginTop: "25px" }} />
      <RegularSeasonPositions teamData={teamData} />
      <Box sx={{ marginTop: "25px" }} />
      <PlayoffPositions teamData={teamData} />
    </Box>
  );
}

const TeamData = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "Team", flex: 1, minWidth: 130 },
    { field: "average", headerName: "Average", flex: 1, type: "number" },
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

  const rows = Object.entries(teamData)
    .map(([_teamName, teamResults]) => {
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
    })
    .sort((a, b) => b.projected_wins - a.projected_wins);

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
          Team Scoring Data
        </Typography>
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

const RegularSeasonPositions = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "Team", flex: 1, minWidth: 130 },
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
        teamName: teamResults.teamName,
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
          Regular Season Projections
        </Typography>
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

const PlayoffPositions = ({ teamData }) => {
  if (teamData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "Team", flex: 1, minWidth: 130 },
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
        teamName: teamResults.teamName,
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

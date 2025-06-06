import React from "react";
import Head from "next/head";
import TitleComponent from "../components/TitleComponent";
import ExpectedWins from "../simulation/expectedWins";
import EnhancedTable from "../ui/components/SortableTable";
import {
  Box,
  Button,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import SimulatorV2 from "../simulation/simulator2";
import { Schedule } from "../simulation/simulator2";

export default function Home() {
  const [historicalData, setHistoricalData] = React.useState([]);
  const [current, setCurrent] = React.useState([]);
  const [currentWithXWins, setCurrentWithXWins] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);
  const [allTimeSchedule, setAllTimeSchedule] = React.useState(null);
  const [allTimeWithXWins, setAllTimeWithXWins] = React.useState(null);
  const [remainingGames, setRemainingGames] = React.useState(null);
  const [currentWeek, setCurrentWeek] = React.useState(null);

  React.useEffect(() => {
    if (schedule !== null) {
      let sim = new ExpectedWins(schedule);
      let xWins = sim.expectedWins();
      // combine the expected wins with the current standings
      let currentStandings = current
        .map((team) => {
          return {
            ...team,
            expectedWins: xWins.find((x) => x.id === team.id).wins,
            diff: team.wins - xWins.find((x) => x.id === team.id).wins,
            percentage: team.wins / (team.wins + team.losses),
          };
        })
        .sort((a, b) => {
          return b.expectedWins - a.expectedWins;
        });
      setCurrentWithXWins(currentStandings);
    }
  }, [current, schedule]);

  React.useEffect(() => {
    if (allTimeSchedule !== null) {
      let sim = new ExpectedWins(allTimeSchedule);
      let xWins = sim.expectedWins();
      // combine the expected wins with the current standings
      let currentStandings = historicalData
        .map((team) => {
          return {
            ...team,
            expectedWins: xWins.find((x) => x.id === team.id).wins,
            diff: team.wins - xWins.find((x) => x.id === team.id).wins,
            percentage: team.wins / (team.wins + team.losses),
          };
        })
        .sort((a, b) => {
          if (b.wins !== a.wins) {
            return b.wins - a.wins;
          }
          return b.totalPoints - a.totalPoints;
        });
      setAllTimeWithXWins(currentStandings);
    }
  }, [allTimeSchedule, historicalData]);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/historical");
      const data = await response.json();
      setHistoricalData(data);
    }

    fetchData();
  }, []);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/schedule?year=all");
      const data = await response.json();
      setAllTimeSchedule(data);
    }

    fetchData();
  }, []);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/schedule");
      const data = await response.json();
      setSchedule(data);
    }

    fetchData();
  }, []);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/current");
      const data = await response.json();
      setCurrent(
        data.sort((a, b) => {
          if (b.wins === a.wins) {
            return b.points - a.points;
            f;
          }
          return b.wins - a.wins;
        })
      );
    }

    fetchData();
  }, []);

  React.useEffect(() => {
    fetch("/api/schedule/")
      .then((response) => response.json())
      .then((data) => {
        const rg = [];
        data.forEach((week, index) => {
          const w = [];
          week.forEach((game) => {
            if (game.completed === false) {
              w.push(game);
            }
          });
          if (w.length > 0) {
            rg.push(w);
          }
        });
        setRemainingGames(rg);

        data.forEach((week, index) => {
          let set = false;
          week.forEach((game) => {
            if (
              game.home_team_final_score > 0 &&
              game.away_team_final_score > 0 &&
              !set
            ) {
              setCurrentWeek(index + 1);
              set = true;
              return;
            }
          });
          if (set) {
            return;
          }
        });
      });
  }, []);

  return (
    <>
      <Head>The League FF</Head>
      <TitleComponent>The League</TitleComponent>
      <ChooseYourDestinyTable
        remainingGames={remainingGames}
        currentWeek={currentWeek}
      />
      {currentWithXWins && currentWithXWins.length > 0 && (
        <EnhancedTable
          rows={currentWithXWins}
          title="Current Standings"
          defaultSort="wins"
        />
      )}
      {allTimeWithXWins && allTimeWithXWins.length > 0 && (
        <EnhancedTable
          rows={allTimeWithXWins}
          title="All Time Standings"
          defaultSort="wins"
        />
      )}
    </>
  );
}

function ChooseYourDestinyTable({ remainingGames, currentWeek }) {
  const [simulator, setSimulator] = React.useState(null);
  const [teamData, setTeamData] = React.useState(null);
  const [teamStats, setTeamStats] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);
  const [cellColors, setCellColors] = React.useState(
    Array(10).fill(Array(4).fill("none"))
  );

  React.useEffect(() => {
    if (teamStats !== null && schedule !== null) {
      const start = new Date().getTime();
      const sim = new SimulatorV2(new Schedule(schedule));
      sim.simulate();
      setTeamData(sim.getTeamData());
      setSimulator(sim);
      console.log("Simulation took " + (new Date().getTime() - start) + " ms");
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

  if (
    remainingGames === null ||
    remainingGames.length === 0 ||
    simulator === null
  ) {
    return <div>Loading...</div>;
  }

  const teams = [];
  remainingGames[0].forEach((game) => {
    teams.push({ name: game.home_team_owner, id: game.home_team_espn_id });
    teams.push({ name: game.away_team_owner, id: game.away_team_espn_id });
  });

  const getOpponent = (teamId, index) => {
    let oppName = "";
    for (let i = 0; i < remainingGames[index].length; i++) {
      if (remainingGames[index][i].home_team_espn_id === teamId) {
        oppName =
          teams.find(
            (team) => team.id === remainingGames[index][i].away_team_espn_id
          )?.name || "";
      } else if (remainingGames[index][i].away_team_espn_id === teamId) {
        oppName =
          teams.find(
            (team) => team.id === remainingGames[index][i].home_team_espn_id
          )?.name || "";
      }
    }
    if (oppName !== "") {
      return (
        oppName
          .split(" ")
          .filter((word) => word !== "")[1]
          .charAt(0)
          .toUpperCase() +
        oppName
          .split(" ")
          .filter((word) => word !== "")[1]
          .slice(1)
      );
    }
    return "NOT FOUND";
  };

  const getOpponentId = (teamId, index) => {
    for (let i = 0; i < remainingGames[index].length; i++) {
      if (remainingGames[index][i].home_team_espn_id === teamId) {
        return remainingGames[index][i].away_team_espn_id;
      } else if (remainingGames[index][i].away_team_espn_id === teamId) {
        return remainingGames[index][i].home_team_espn_id;
      }
    }
    return -1;
  };

  const handleCellClick = (teamId, weekIndex) => {
    const opponentId = getOpponentId(teamId, weekIndex);
    setCellColors((prevColors) => {
      const withTeamIdColors = prevColors.map((row, i) =>
        row.map((color, j) => {
          if (teams[i].id === teamId && j === weekIndex) {
            if (color === "none") {
              simulator.addFilter(weekIndex + currentWeek + 1, teamId);
              setTeamData(simulator.getTeamData());
              return "lightgreen";
            }
            if (color === "lightgreen") {
              simulator.removeFilter(weekIndex + currentWeek + 1, teamId);
              simulator.addFilter(weekIndex + currentWeek + 1, opponentId);
              setTeamData(simulator.getTeamData());
              return "red";
            }
            simulator.removeFilter(weekIndex + currentWeek + 1, opponentId);
            setTeamData(simulator.getTeamData());
            return "none";
          }
          return color;
        })
      );
      const finalizedColors = withTeamIdColors.map((row, i) =>
        row.map((color, j) => {
          if (teams[i].id === opponentId && j === weekIndex) {
            if (color === "none") {
              return "red";
            }
            if (color === "red") {
              return "lightgreen";
            }
            return "none";
          }
          return color;
        })
      );
      return finalizedColors;
    });
  };

  if (simulator === null) {
    return <div>Loading...</div>;
  }

  const lastPlaceByID = (id) => {
    if (teamData === null || teamData === undefined) {
      return 0;
    }
    for (let i = 0; i < teamData.length; i++) {
      if (teamData[i].teamId === id) {
        return teamData[i].lastPlaceOdds;
      }
    }
    return 0.0;
  };

  const playoffOddsById = (id) => {
    if (teamData === null || teamData === undefined) {
      return 0;
    }
    for (let i = 0; i < teamData.length; i++) {
      if (teamData[i].teamId === id) {
        return teamData[i].playoffOdds;
      }
    }
    return 0.0;
  };

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        padding: "15px",
      }}
    >
      <Typography
        variant="h5"
        sx={{ paddingTop: "15px", paddingBottom: "15px" }}
      >
        Choose Your Destiny
      </Typography>
      <Typography
        variant="body1"
        sx={{ paddingTop: "15px", paddingBottom: "15px" }}
      >
        There are {remainingGames.length} week
        {remainingGames.length > 1 ? "s" : ""} to be played. Here are the
        matchups that will determine the final standings.
      </Typography>
      <Box
        sx={{
          display: "flex",
          flexDirection: "row",
          alignItems: "center",
          justifyContent: "center",
          padding: "15px",
        }}
      >
        <Button
          onClick={() => {
            simulator.removeAllFilters();
            setTeamData(simulator.getTeamData());
            setCellColors(Array(10).fill(Array(4).fill("none")));
          }}
          variant="contained"
          sx={{ marginRight: "10px" }}
        >
          Reset
        </Button>
        <Typography variant="body1">
          (Sample Size: {simulator.filteredResults.length})
        </Typography>
      </Box>
      <TableContainer sx={{ paddingTop: "15px", paddingBottom: "15px" }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell align="right" />
              <TableCell align="right">Playoffs</TableCell>
              <TableCell align="right">Last Place</TableCell>
              {Array.from({ length: remainingGames.length }, (_, index) => (
                <TableCell key={index} align="right">
                  Week {currentWeek + index + 1}
                </TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {teams.map((team, teamIndex) => (
              <TableRow key={teamIndex}>
                <TableCell align="right">{team.name}</TableCell>
                <TableCell align="right">
                  {(100 * playoffOddsById(team.id)).toFixed(2)} %
                </TableCell>
                <TableCell align="right">
                  {(100 * lastPlaceByID(team.id)).toFixed(2)} %
                </TableCell>
                {remainingGames.map((week, weekIndex) => (
                  <TableCell
                    key={weekIndex}
                    align="right"
                    sx={{ backgroundColor: cellColors[teamIndex][weekIndex] }}
                    onClick={() => {
                      handleCellClick(team.id, weekIndex);
                    }}
                  >
                    {getOpponent(team.id, weekIndex)}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}

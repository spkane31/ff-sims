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

import {
  createTheme,
  ThemeProvider,
  alpha,
  getContrastRatio,
} from "@mui/material/styles";

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
            if (game.completed === false) {
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
      {remainingGames && remainingGames.length > 0 && currentWeek && (
        <ChooseYourDestinyTable
          remainingGames={remainingGames}
          currentWeek={currentWeek}
        />
      )}
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
  const [variant, setVariant] = React.useState("contained");

  const teams = [];
  remainingGames[0].forEach((game) => {
    teams.push({ name: game.home_team_owner, id: game.home_team_espn_id });
    teams.push({ name: game.away_team_owner, id: game.away_team_espn_id });
  });

  const handleButtonClick = () => {
    setVariant((prevVariant) =>
      prevVariant === "contained" ? "outlined" : "contained"
    );
  };

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

  return (
    <Box>
      <Typography
        variant="h5"
        sx={{ paddingTop: "15px", paddingBottom: "15px" }}
      >
        Choose Your Destiny
      </Typography>
      <Typography
        variant="p"
        sx={{ paddingTop: "15px", paddingBottom: "15px" }}
      >
        There are {remainingGames.length} week
        {remainingGames.length > 1 ? "s" : ""} to be played. Here are the
        matchups that will determine the final standings.
      </Typography>
      <TableContainer sx={{ paddingTop: "15px", paddingBottom: "15px" }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell align="right" />
              {Array.from({ length: remainingGames.length }, (_, index) => (
                <TableCell key={index} align="right">
                  Week {currentWeek + index - remainingGames.length + 1}
                </TableCell>
              ))}
              <TableCell align="right">Playoffs</TableCell>
              <TableCell align="right">Last Place</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {teams.map((team, index) => (
              <TableRow key={index}>
                <TableCell align="right">{team.name}</TableCell>
                {remainingGames.map((week) => {
                  console.log(week);
                })}
                {Array.from({ length: 5 }, (_, index) => (
                  <TableCell key={index} align="right">
                    <SwapButton text={`vs ${getOpponent(team.id, index)}`} />
                  </TableCell>
                ))}
                <TableCell align="right">Playoffs</TableCell>
                <TableCell align="right">Last Place</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}

function SwapButton({ text }) {
  // const [color, setColor] = React.useState("white");
  // const [textColor, setTextColor] = React.useState("black");
  const [buttonState, setButtonState] = React.useState(0);
  const [theme, setTheme] = React.useState(neutralTheme);

  // const colors = ["white", "green", "red"];
  // const textColors = ["black", "white", "white"];
  const themes = [neutralTheme, winTheme, loseTheme];

  const handleButtonClick = () => {
    // setColor(colors[buttonState]);
    // setTextColor(textColors[buttonState]);
    setButtonState((prevState) => (prevState + 1) % themes.length);
    setTheme(themes[buttonState]);
  };

  return (
    <ThemeProvider theme={theme}>
      <Button
        // style={{ backgroundColor: color, color: textColor }}
        onClick={handleButtonClick}
        variant="contained"
      >
        {text}
      </Button>
    </ThemeProvider>
  );
}
const greenBase = "#00FF00";
const greenMain = alpha(greenBase, 0.7);
const winTheme = createTheme({
  palette: {
    green: {
      main: greenMain,
      light: alpha(greenBase, 0.5),
      dark: alpha(greenBase, 0.9),
      contrastText: getContrastRatio(greenMain, "#fff") > 4.5 ? "#fff" : "#111",
    },
  },
});

const redBase = "#FF0000";
const redMain = alpha(redBase, 0.7);
const loseTheme = createTheme({
  palette: {
    red: {
      main: redMain,
      light: alpha(redBase, 0.5),
      dark: alpha(redBase, 0.9),
      contrastText: getContrastRatio(redMain, "#fff") > 4.5 ? "#fff" : "#111",
    },
  },
});

const neutralTheme = createTheme({
  palette: {
    common: {
      white: "#FFFFFF",
    },
  },
});

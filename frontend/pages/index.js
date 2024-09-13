import React from "react";
import Head from "next/head";
import { Box, Typography } from "@mui/material";
import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";
import ExpectedWins from "../simulation/expectedWins";

const paddingAmount = "15px";

export default function Home() {
  const [historicalData, setHistoricalData] = React.useState([]);
  const [current, setCurrent] = React.useState([]);
  const [schedule, setSchedule] = React.useState(null);
  const [teamStats, setTeamStats] = React.useState(null);
  const [allTimeSchedule, setAllTimeSchedule] = React.useState(null);

  React.useEffect(() => {
    if (teamStats !== null && schedule !== null) {
      let sim = new ExpectedWins(teamStats, schedule);
      let xWins = sim.expectedWins();
      // combine the expected wins with the current standings
      let currentStandings = current
        .map((team) => {
          return {
            ...team,
            expectedWins: xWins.find((x) => x.id === team.id).wins,
          };
        })
        .sort((a, b) => {
          return b.expectedWins - a.expectedWins;
        });
      setCurrent(currentStandings);
    }
  }, [teamStats, schedule]);

  React.useEffect(() => {
    if (teamStats !== null && allTimeSchedule !== null) {
      let sim = new ExpectedWins(teamStats, allTimeSchedule);
      let xWins = sim.expectedWins();
      // combine the expected wins with the current standings
      let currentStandings = historicalData
        .map((team) => {
          return {
            ...team,
            expectedWins: xWins.find((x) => x.id === team.id).wins.toFixed(2),
          };
        })
        .sort((a, b) => {
          return b.expectedWins - a.expectedWins;
        });
      setHistoricalData(currentStandings);
    }
  }, [teamStats, allTimeSchedule]);

  React.useEffect(() => {
    fetch("/api/teams")
      .then((res) => res.json())
      .then((data) => {
        setTeamStats(data);
      });
  }, []);

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

  return (
    <>
      <Box
        sx={{
          padding: paddingAmount,
        }}
      />
      <Head>The League FF</Head>
      <TitleComponent>The League</TitleComponent>
      <Typography variant="h6">Current Standings</Typography>
      <Box sx={{ padding: paddingAmount }} />
      <Table data={current} />
      <Box sx={{ padding: paddingAmount }} />
      <Typography variant="h6">All Time Standings</Typography>
      <Box sx={{ padding: paddingAmount }} />
      <Table data={historicalData} />
      <Box sx={{ padding: paddingAmount }} />
    </>
  );
}

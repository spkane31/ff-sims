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
  const [currentWithXWins, setCurrentWithXWins] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);
  const [allTimeSchedule, setAllTimeSchedule] = React.useState(null);
  const [allTimeWithXWins, setAllTimeWithXWins] = React.useState(null);

  // Log the frontend request
  React.useEffect(() => {
    fetch("/api/log", {
      method: "POST",
      body: JSON.stringify({
        endpoint: "/",
        method: "GET",
      }),
      headers: {
        "Content-Type": "application/json",
      },
    });
  }, []);

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
            expectedWins: xWins.find((x) => x.id === team.id).wins.toFixed(2),
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
      <Table data={currentWithXWins} />
      <Box sx={{ padding: paddingAmount }} />
      <Typography variant="h6">All Time Standings</Typography>
      <Box sx={{ padding: paddingAmount }} />
      <Table data={allTimeWithXWins} />
      <Box sx={{ padding: paddingAmount }} />
    </>
  );
}

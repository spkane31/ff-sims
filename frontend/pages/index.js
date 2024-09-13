import React from "react";
import Head from "next/head";
import { Box, Typography } from "@mui/material";
import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";
import Simulator from "../simulation/simulator";

const paddingAmount = "15px";

export default function Home() {
  const [historicalData, setHistoricalData] = React.useState([]);
  const [current, setCurrent] = React.useState([]);
  const [schedule, setSchedule] = React.useState(null);
  const [simulator, setSimulator] = React.useState(null);
  const [teamStats, setTeamStats] = React.useState(null);

  React.useEffect(() => {
    if (teamStats !== null && schedule !== null) {
      let sim = new Simulator(teamStats, schedule);
      let xWins = sim.expectedWins();
      // combine the expected wins with the current standings
      let currentStandings = current
        .map((team) => {
          console.log(team);
          let expectedWins = xWins[team.name];
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

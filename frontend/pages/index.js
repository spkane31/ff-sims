import React from "react";
import Head from "next/head";
import { Box, Typography } from "@mui/material";
import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";

const paddingAmount = "15px";

export default function Home() {
  const [historicalData, setHistoricalData] = React.useState([]);
  const [current, setCurrent] = React.useState([]);

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

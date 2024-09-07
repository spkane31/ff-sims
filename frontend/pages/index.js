import React from "react";
import Head from "next/head";
import { Box, Typography } from "@mui/material";
import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";

const paddingAmount = "15px";

export default function Home() {
  const [data, setData] = React.useState([]);
  const [current, setCurrent] = React.useState([]);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/historical");
      const data = await response.json();
      setData(data);
    }

    fetchData();
  }, []);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/current");
      const data = await response.json();
      setCurrent(data);
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
      <Table data={data} />
      <Box sx={{ padding: paddingAmount }} />
    </>
  );
}

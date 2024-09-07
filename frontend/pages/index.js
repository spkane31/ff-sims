import React from "react";
import Head from "next/head";
import { Box, Typography } from "@mui/material";
import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";

export default function Home() {
  const [data, setData] = React.useState([]);

  React.useEffect(() => {
    async function fetchData() {
      const response = await fetch("/api/historical");
      const data = await response.json();
      setData(data);
    }

    fetchData();
  }, []);

  return (
    <>
      <Box
        sx={{
          padding: "10px",
        }}
      />
      <Head>The League FF</Head>
      <TitleComponent>The League</TitleComponent>
      <Typography variant="h6">All Time Standings</Typography>
      <Box sx={{ padding: "15px" }} />
      <Table data={data} />
      <Box sx={{ padding: "15px" }} />
    </>
  );
}

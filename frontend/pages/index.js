import React from "react";
import Head from "next/head";
import { Box, Typography } from "@mui/material";
import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";

export default function Home() {
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
      <Table />
    </>
  );
}

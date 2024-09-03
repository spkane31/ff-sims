import Head from "next/head";
import { Box, Typography } from "@mui/material";

import Table from "../components/Table";
import TitleComponent from "../components/TitleComponent";

import React from "react";
import { styled } from "@mui/system";

const Container = styled("div")({
  minHeight: "100vh",
  padding: "0 0.5rem",
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
});

export default function Home() {
  return (
    <Container>
      <Head>The League FF</Head>
      <Box
        sx={{
          padding: "5rem 0",
          flex: 1,
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          alignItems: "center",
          overflowX: "auto",
        }}
      >
        <TitleComponent>The League</TitleComponent>
        <Typography>All Time Standings</Typography>
        <Box sx={{ paddingBottom: "15px" }} />
        <Table />
      </Box>
    </Container>
  );
}

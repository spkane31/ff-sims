import Head from "next/head";
import { Box } from "@mui/material";

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
      <Head>
        <title>The League FF</title>
      </Head>
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
        <p>All Time Standings</p>
        <Table />
      </Box>
    </Container>
  );
}

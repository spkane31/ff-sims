import Head from "next/head";
import { ThemeProvider, Box } from "@mui/material";

import Table from "../components/Table";
import Layout from "../components/Layout";
import theme from "../components/theme";
import TitleComponent from "../components/TitleComponent";

import React from "react";
import { styled } from "@mui/system";

const Container = styled("div")({
  minHeight: "100vh",
  padding: "0 0.5rem",
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  alignItems: "center",
});

export default function Home() {
  return (
    <ThemeProvider theme={theme}>
      <Layout>
        <Container>
          <Head>
            <title>The League FF</title>
            <link rel="icon" href="/favicon.ico" />
          </Head>
          <Box
            sx={{
              padding: "5rem 0",
              flex: 1,
              display: "flex",
              flexDirection: "column",
              justifyContent: "center",
              alignItems: "center",
            }}
          >
            <TitleComponent>The League</TitleComponent>
            <p>All Time Standings</p>
            <Table />
          </Box>
        </Container>
      </Layout>
    </ThemeProvider>
  );
}

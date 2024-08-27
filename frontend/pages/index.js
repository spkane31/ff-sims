import Head from "next/head";
import { ThemeProvider, Box } from "@mui/material";

import styles from "../styles/Home.module.css";
import Table from "../components/Table";
import Layout from "../components/Layout";
import theme from "../components/theme";
import TitleComponent from "../components/TitleComponent";

export default function Home() {
  return (
    <ThemeProvider theme={theme}>
      <Layout>
        <div className={styles.container}>
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
        </div>
      </Layout>
    </ThemeProvider>
  );
}

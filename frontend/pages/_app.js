import React from "react";
import MyToolbar from "../components/Toolbar";
import Footer from "../components/Footer";
import { ThemeProvider, Box } from "@mui/material";
import theme from "../components/theme";

import { styled } from "@mui/system";

const Container = styled("div")({
  minHeight: "100vh",
  padding: "0 0.5rem",
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
});

function MyApp({ Component, pageProps }) {
  return (
    <ThemeProvider theme={theme}>
      <Box
        sx={{
          paddingLeft: "2%",
          paddingRight: "2%",
          flex: 1,
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          alignItems: "center",
          overflowX: "auto",
        }}
      >
        <MyToolbar />
        <Component {...pageProps} />
        <Footer />
      </Box>
    </ThemeProvider>
  );
}

export default MyApp;

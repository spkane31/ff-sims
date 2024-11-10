import React from "react";
import MyToolbar from "../components/Toolbar";
import Footer from "../components/Footer";
import { ThemeProvider, Box } from "@mui/material";
import theme from "../components/theme";
import "./global.css";

function MyApp({ Component, pageProps }) {
  // Log the frontend request
  React.useEffect(() => {
    console.log("Logging request for ", window.location.href);
    fetch("/api/log", {
      method: "POST",
      body: JSON.stringify({
        endpoint: window.location.href,
        method: "GET",
        userAgent: window.navigator.userAgent,
      }),
      headers: {
        "Content-Type": "application/json",
      },
    });
  }, []);

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

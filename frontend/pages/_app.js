import React from "react";
import MyToolbar from "../components/Toolbar";
import Footer from "../components/Footer";
import { ThemeProvider } from "@mui/material";
import theme from "../components/theme";

function MyApp({ Component, pageProps }) {
  return (
    <ThemeProvider theme={theme}>
      <MyToolbar />
      <Component {...pageProps} />
      <Footer />
    </ThemeProvider>
  );
}

export default MyApp;

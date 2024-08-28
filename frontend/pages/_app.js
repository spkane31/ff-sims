import MyToolbar from "../components/Toolbar";
import { ThemeProvider } from "@mui/material";
import theme from "../components/theme";

function MyApp({ Component, pageProps }) {
  return (
    <ThemeProvider theme={theme}>
      <MyToolbar />
      <Component {...pageProps} />
    </ThemeProvider>
  );
}

export default MyApp;

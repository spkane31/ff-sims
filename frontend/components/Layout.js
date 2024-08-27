// components/Layout.js

import React from "react";
import { Container, Typography, Box, AppBar, Toolbar } from "@mui/material";

const Layout = ({ children }) => {
  return (
    <Box display="flex" flexDirection="column" minHeight="100vh">
      <AppBar position="static">
        <Toolbar>
          <Typography variant="h6">The League Statistics</Typography>
        </Toolbar>
      </AppBar>

      <Container component="main" sx={{ flex: 1 }}>
        {children}
      </Container>

      <Box
        component="footer"
        sx={{
          backgroundColor: "#228B22",
          padding: 2,
          textAlign: "center",
          mt: "auto",
        }}
      >
        <Typography variant="body1" color="white">
          Powered by Male Friendship
        </Typography>
      </Box>
    </Box>
  );
};

export default Layout;

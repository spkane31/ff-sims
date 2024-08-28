// components/Layout.js

import React from "react";
import {
  Container,
  Typography,
  Box,
  AppBar,
  Toolbar,
  IconButton,
  Drawer,
  List,
  ListItem,
  ListItemText,
} from "@mui/material";
import MenuIcon from "@mui/icons-material/Menu";

const Layout = ({ children }) => {
  const [isDrawerOpen, setIsDrawerOpen] = React.useState(false);
  const [isMobile, setIsMobile] = React.useState(false);

  const toggleDrawer = () => {
    setIsDrawerOpen(!isDrawerOpen);
  };

  return (
    <Box display="flex" flexDirection="column" minHeight="100vh">
      <AppBar position="static">
        <Toolbar>
          <Typography variant="h6">The League Statistics</Typography>
          {isMobile && (
            <Box sx={{ flexGrow: 1 }}>
              <IconButton
                edge="end"
                color="inherit"
                aria-label="menu"
                onClick={toggleDrawer}
              >
                <MenuIcon />
              </IconButton>
            </Box>
          )}
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

      <Drawer anchor="right" open={isDrawerOpen} onClose={toggleDrawer}>
        <List>
          <ListItem component="a" href="/">
            <ListItemText primary="Home" />
          </ListItem>
          <ListItem component="a" href="/simulations">
            <ListItemText primary="Simulations" />
          </ListItem>
          <ListItem component="a" href="/other">
            <ListItemText primary="Draft/Trades/Other" />
          </ListItem>
        </List>
      </Drawer>
    </Box>
  );
};

export default Layout;

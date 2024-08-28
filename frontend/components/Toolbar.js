import React, { useState } from "react";
import {
  AppBar,
  Toolbar,
  Typography,
  IconButton,
  Link,
  Menu,
  MenuItem,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import MenuIcon from "@mui/icons-material/Menu";

const MyToolbar = () => {
  const [anchorEl, setAnchorEl] = useState(null);
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));

  const handleMenuOpen = (event) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  return (
    <AppBar position="static">
      <Toolbar>
        {isMobile ? (
          <>
            <Typography variant="h6" style={{ flexGrow: 1 }}>
              The League
            </Typography>
            <IconButton
              edge="end"
              color="inherit"
              aria-label="menu"
              onClick={handleMenuOpen}
            >
              <MenuIcon />
            </IconButton>
            <Menu
              anchorEl={anchorEl}
              open={Boolean(anchorEl)}
              onClose={handleMenuClose}
              slotProps={{
                style: {
                  width: "auto",
                },
              }}
            >
              <MenuItem onClick={handleMenuClose} component={Link} href="/">
                Home
              </MenuItem>
              <MenuItem
                onClick={handleMenuClose}
                component={Link}
                href="/simulations"
              >
                Simulations
              </MenuItem>
              <MenuItem onClick={handleMenuClose} component={Link} href="/data">
                Data
              </MenuItem>
            </Menu>
          </>
        ) : (
          <>
            <Typography variant="h6" style={{ flexGrow: 1 }}>
              The League
            </Typography>
            <div style={{ display: "flex" }}>
              <Link
                href="/"
                style={{
                  textDecoration: "none",
                  color: "white",
                  margin: "0 10px",
                }}
              >
                Home
              </Link>
              <Link
                href="/simulations"
                style={{
                  textDecoration: "none",
                  color: "white",
                  margin: "0 10px",
                }}
              >
                Simulations
              </Link>
              <Link
                href="/data"
                style={{
                  textDecoration: "none",
                  color: "white",
                  margin: "0 10px",
                }}
              >
                Data
              </Link>
            </div>
          </>
        )}
      </Toolbar>
    </AppBar>
  );
};

export default MyToolbar;

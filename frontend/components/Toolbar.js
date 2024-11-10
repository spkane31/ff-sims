import React, { useState } from "react";
import {
  AppBar,
  Toolbar,
  IconButton,
  Link,
  Menu,
  MenuItem,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import MenuIcon from "@mui/icons-material/Menu";

const pages = [
  {
    name: "Home",
    href: "/",
  },
  {
    name: "Simulations",
    href: "/simulations",
  },
  {
    name: "Schedule",
    href: "/schedule",
  },
  {
    name: "Teams",
    href: "/teams",
  },
  {
    name: "Draft",
    href: "/draft",
  },
  {
    name: "Draft Board",
    href: "/draft-grid",
  },
  {
    name: "Players",
    href: "/players",
  },
  {
    name: "Transactions",
    href: "/transactions",
  },
  {
    name: "Data",
    href: "/data",
  },
];

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
            <Link
              href="/"
              variant="h6"
              style={{
                flexGrow: 1,
                textDecoration: "none",
                color: "white",
                margin: "0 10px",
              }}
            >
              The League
            </Link>
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
              {pages.map((page) => (
                <MenuItem
                  key={page.name}
                  onClick={handleMenuClose}
                  component={Link}
                  href={page.href}
                >
                  {page.name}
                </MenuItem>
              ))}
            </Menu>
          </>
        ) : (
          <>
            <Link
              href="/"
              variant="h6"
              style={{
                flexGrow: 1,
                textDecoration: "none",
                color: "white",
                margin: "0 10px",
              }}
            >
              The League
            </Link>
            <div style={{ display: "flex" }}>
              {pages.map((page) => (
                <Link
                  key={page.name}
                  href={page.href}
                  style={{
                    textDecoration: "none",
                    color: "white",
                    margin: "0 10px",
                  }}
                >
                  {page.name}
                </Link>
              ))}
            </div>
          </>
        )}
      </Toolbar>
    </AppBar>
  );
};

export default MyToolbar;

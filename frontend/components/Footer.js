import React from "react";
import { AppBar, Toolbar, Typography } from "@mui/material";

const Footer = () => {
  return (
    <AppBar position="static">
      <Toolbar style={{ justifyContent: "center" }}>
        <Typography
          variant="subtitle1"
          style={{ flexGrow: 1, textAlign: "center" }}
        >
          Powered by Male Friendship
        </Typography>
      </Toolbar>
    </AppBar>
  );
};

export default Footer;

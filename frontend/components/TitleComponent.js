// src/TitleComponent.js

import React from "react";
import { Typography, Link } from "@mui/material";

const TitleComponent = ({ children }) => {
  return (
    <Typography
      variant="h1"
      component="h1"
      sx={{
        fontSize: "4rem",
        textAlign: "center",
        color: "primary.main",
        paddingBottom: "15px",
        paddingTop: "15px",
      }}
    >
      {children}
    </Typography>
  );
};

export default TitleComponent;

// src/TitleComponent.js

import React from "react";
import { Typography, Link } from "@mui/material";

const TitleComponent = ({ children }) => {
  return (
    <Typography
      variant="h1"
      component="h1"
      sx={{
        margin: 0,
        lineHeight: 1.15,
        fontSize: "4rem",
        textAlign: "center",
      }}
    >
      <Link
        href="/"
        sx={{
          textDecoration: "none",
          "&:hover": {
            textDecoration: "underline",
          },
          "&:focus": {
            textDecoration: "underline",
          },
          "&:active": {
            textDecoration: "underline",
          },
        }}
      >
        {children}
      </Link>
    </Typography>
  );
};

export default TitleComponent;

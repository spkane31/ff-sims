import * as React from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Paper, Box, useMediaQuery, useTheme } from "@mui/material";

import historical from "../data/historical.json";

const columns = [
  { field: "owner", headerName: "Owner" },
  {
    field: "total_points_for",
    headerName: "Total Points",
    type: "number",
    sortable: true,
    valueGetter: (_value, row) => `${row.total_points_for.toLocaleString()}`,
  },
  {
    field: "wins",
    headerName: "Total Wins",
    type: "number",
    sortable: true,
  },
  {
    field: "losses",
    headerName: "Total Losses",
    type: "number",
  },
  {
    field: "record",
    headerName: "Record",
    sortable: true,
    valueGetter: (_value, row) => {
      return `${row.record.toFixed(3)}`;
    },
  },
];

export default function Table() {
  const theme = useTheme();

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 500,
          minHeight: 400,
        }}
      >
        <DataGrid
          rows={historical}
          columns={columns}
          autosizeOnMount
          hideFooter
        />
      </Paper>
    </Box>
  );
}

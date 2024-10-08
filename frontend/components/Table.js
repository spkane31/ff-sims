import * as React from "react";
import { DataGrid } from "@mui/x-data-grid";
import { Paper, Box, useTheme, Link } from "@mui/material";

const columns = [
  {
    field: "owner",
    headerName: "Owner",
    minWidth: 100,
    flex: 1,
    renderCell: (param) => (
      <Link href={`/team/${param.row.id}`}>{`${param.row.owner}`}</Link>
    ),
  },
  {
    field: "points",
    headerName: "Total Points",
    type: "number",
    sortable: true,
    valueGetter: (_value, row) => `${row.points.toFixed(2).toLocaleString()}`,
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
    field: "expectedWins",
    headerName: "Expected Wins",
    type: "number",
  },
  {
    field: "diff",
    headerName: "Difference",
    type: "number",
    valueGetter: (_value, row) => {
      if (row.expectedWins === undefined) {
        return "0.00";
      }
      return (row.wins - row.expectedWins).toFixed(2);
    },
  },
  {
    field: "record",
    headerName: "Percentage",
    sortable: true,
    valueGetter: (_value, row) => {
      if (row.wins === 0 && row.losses === 0) {
        return "0.000";
      }
      return `${(row.wins / (row.wins + row.losses)).toFixed(3)}`;
    },
  },
];

export default function Table({ data }) {
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
        <DataGrid rows={data} columns={columns} autosizeOnMount hideFooter />
      </Paper>
    </Box>
  );
}

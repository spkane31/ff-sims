import * as React from "react";
import { Paper, Link } from "@mui/material";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";

const columns = [
  {
    field: "owner",
    headerName: "",
    minWidth: 100,
    flex: 1,
    renderCell: (param) => (
      <Link href={`/team/${param.row.id}`}>{`${param.row.owner}`}</Link>
    ),
  },
  {
    field: "points",
    headerName: "Points",
    type: "number",
    sortable: true,
    valueGetter: (_value, row) => `${row.points.toFixed(2).toLocaleString()}`,
  },
  {
    field: "wins",
    headerName: "Wins",
    type: "number",
    sortable: true,
  },
  {
    field: "losses",
    headerName: "Losses",
    type: "number",
  },
  {
    field: "expectedWins",
    headerName: "xWins",
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

export default function DenseTable({ data }) {
  if (data === null || data === undefined) {
    return;
  }

  return (
    <TableContainer component={Paper}>
      <Table size="small">
        <TableHead>
          <TableRow>
            {columns.map((column) => (
              <TableCell key={column.field} align="right">
                {column.headerName}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {data.map((row) => (
            <TableRow
              key={row.id}
              sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
            >
              <TableCell component="th" scope="row">
                {row.owner}
              </TableCell>
              <TableCell align="right">{row.points.toFixed(2)}</TableCell>
              <TableCell align="right">{row.wins}</TableCell>
              <TableCell align="right">{row.losses}</TableCell>
              <TableCell align="right">{row.expectedWins}</TableCell>
              <TableCell align="right">
                {(row.wins - row.expectedWins).toFixed(2)}
              </TableCell>
              <TableCell align="right">
                {(row.wins / (row.wins + row.losses)).toFixed(3)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

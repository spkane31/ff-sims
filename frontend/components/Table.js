import * as React from "react";
import { DataGrid } from "@mui/x-data-grid";

import historical from "../data/historical.json";

const columns = [
  { field: "owner", headerName: "Owner", width: 150 },
  {
    field: "total_points_for",
    headerName: "Total Points",
    type: "number",
    sortable: true,
    valueGetter: (_value, row) => `${row.total_points_for.toFixed(2)}`,
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
  return (
    <div style={{ width: "100%", overflowX: "auto" }}>
      <DataGrid rows={historical.slice(0, 10)} columns={columns} autoHeight />
    </div>
  );
}

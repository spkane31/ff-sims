import * as React from "react";
import { DataGrid } from "@mui/x-data-grid";

const columns = [
  // { field: "id", headerName: "ID", width: 0 },
  { field: "teamName", headerName: "Team Name", width: 150 },
  { field: "owner", headerName: "Owner", width: 150 },
  {
    field: "totalPoints",
    headerName: "Total Points",
    type: "number",
    width: 150,
    sortable: true,
    valueGetter: (value, row) => `${row.totalPoints.toFixed(2)}`,
  },
  {
    field: "totalWins",
    headerName: "Total Wins",
    type: "number",
    width: 150,
    sortable: true,
  },
  {
    field: "totalLosses",
    headerName: "Total Losses",
    type: "number",
    width: 150,
  },
  {
    field: "record",
    headerName: "Record",
    description: "This column has a value getter and is not sortable.",
    sortable: false,
    width: 160,
    valueGetter: (value, row) => {
      if (row.totalWins === undefined || row.totalWins === 0) {
        return "0.000";
      }
      return `${(row.totalWins / (row.totalWins + row.totalLosses)).toFixed(
        3
      )}`;
    },
  },
];

const rows = [
  {
    id: 1,
    teamName: "Ketchup and Mostert",
    owner: "Connor Brand",
    totalPoints: 1836.7,
    totalWins: 6,
    totalLosses: 8,
  },
  {
    id: 2,
    teamName: "Christian Mingle",
    owner: "Nick Toth",
    totalPoints: 1841.0,
    totalWins: 6,
    totalLosses: 8,
  },
  {
    id: 3,
    teamName: "Loading...",
    owner: "Kyle Burns",
    totalPoints: 2059.82,
    totalWins: 10,
    totalLosses: 4,
  },
  {
    id: 4,
    teamName: "The Glass Legs",
    owner: "Kevin Dailey",
    totalPoints: 1791.4,
    totalWins: 5,
    totalLosses: 9,
  },
  {
    id: 5,
    teamName: "The Nut Dumpster",
    owner: "Sean Kane",
    totalPoints: 2124.58,
    totalWins: 8,
    totalLosses: 6,
  },
  {
    id: 6,
    teamName: "Daddy Doepker",
    owner: "Josh Doepker",
    totalPoints: 2064.38,
    totalWins: 10,
    totalLosses: 4,
  },
  {
    id: 7,
    teamName: "Chef Hans",
    owner: "Mitch Lichtinger",
    totalPoints: 1759.34,
    totalWins: 4,
    totalLosses: 10,
  },
  {
    id: 8,
    teamName: "Walker Texas Nutter",
    owner: "Jack Aldridge",
    totalPoints: 1841.14,
    totalWins: 7,
    totalLosses: 7,
  },
  {
    id: 9,
    teamName: "Brock'd Up",
    owner: "Ethan Moran",
    totalPoints: 1926.8,
    totalWins: 9,
    totalLosses: 5,
  },
];

export default function Table() {
  return (
    <div style={{ height: "100%", width: "100%" }}>
      <DataGrid
        rows={rows}
        columns={columns}
        // initialState={{
        //   pagination: {
        //     paginationModel: { page: 0, pageSize: 10 },
        //   },
        // }}
        // pageSizeOptions={[10]}
        // checkboxSelection
        sx={{ overflow: "clip" }}
      />
    </div>
  );
}

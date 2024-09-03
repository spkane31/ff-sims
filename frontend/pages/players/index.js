import React from "react";
import { Box, Paper, Typography } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

const columns = [
  {
    field: "year",
    headerName: "Year",
    type: "number",
    sortable: true,
    valueGetter: (row) => `${row}`,
  },
  {
    field: "player_name",
    headerName: "Player",
    sortable: true,
  },
  {
    field: "total_projected_points",
    headerName: "Projected Points",
    type: "number",
    sortable: true,
  },
  {
    field: "total_actual_points",
    headerName: "Actual Points",
    type: "number",
    sortable: true,
  },
  {
    field: "diff",
    headerName: "Difference",
    type: "number",
    sortable: true,
  },
];

// Create a functional component for the page
const Data = () => {
  const [players, setPlayers] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/boxscoreplayers?year=2023")
      .then((res) => res.json())
      .then((data) => {
        const dataWithID = data
          .map((player, index) => {
            return {
              ...player,
              id: index,
              diff: player.total_actual_points - player.total_projected_points,
            };
          })
          .sort((a, b) => b.diff - a.diff);

        setPlayers(dataWithID);
      });
  }, []);

  if (players === null) {
    return <div>Loading...</div>;
  }

  return (
    <Box
      sx={{
        padding: "5rem 0",
        flex: 1,
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
        overflowX: "auto",
        paddingLeft: "5%",
        paddingRight: "5%",
      }}
    >
      <Box sx={{ marginTop: "25px" }} />
      <Typography variant="h4" sx={{ textAlign: "center" }}>
        Player Standings
      </Typography>
      <Box sx={{ paddingBottom: "15px" }} />
      <DataGrid
        rows={players}
        columns={columns}
        autosizeOnMount
        hideFooter
        sx={{
          minWidth: "500px",
        }}
      />
    </Box>
  );
};

export default Data;

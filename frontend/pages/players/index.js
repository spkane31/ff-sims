import React from "react";
import MenuItem from "@mui/material/MenuItem";
import {
  Box,
  Paper,
  Typography,
  InputLabel,
  FormControl,
  Select,
} from "@mui/material";
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
    width: 200,
  },
  {
    field: "player_position",
    headerName: "Position",
    sortable: true,
  },
  {
    field: "total_actual_points",
    headerName: "Points",
    type: "number",
    sortable: true,
  },
  {
    field: "total_projected_points",
    headerName: "Projected Points",
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
  const [year, setYear] = React.useState("All");

  const getURL = (year) => {
    if (year === "All") {
      return `/api/boxscoreplayers`;
    }
    return `/api/boxscoreplayers?year=${year}`;
  };

  React.useEffect(() => {
    fetch(getURL(year))
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
          .sort((a, b) => b.total_actual_points - a.total_actual_points);

        setPlayers(dataWithID);
      });
  }, [year]);

  const handleChange = (event) => {
    setYear(event.target.value);
  };

  const currentYear = new Date().getFullYear();
  const years = Array.from(
    { length: currentYear - 2017 + 1 },
    (_, index) => 2017 + index
  ).reverse();

  years.unshift("All");

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

      <Paper
        sx={{
          padding: "10px",
          display: "flex",
          justifyContent: "center",
          alignItems: "center",
        }}
      >
        <Typography variant="h6">Year:</Typography>
        <Box
          sx={{
            padding: "10px",
          }}
        >
          <FormControl fullWidth>
            <Select value={year} onChange={handleChange}>
              {years.map((year) => (
                <MenuItem key={year} value={year}>
                  {year}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>
      </Paper>

      <Box sx={{ marginTop: "15px" }} />
      <Box sx={{ paddingBottom: "15px" }} />
      <DataGrid rows={players} columns={columns} autosizeOnMount hideFooter />
    </Box>
  );
};

export default Data;

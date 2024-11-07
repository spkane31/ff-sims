import React from "react";
import {
  Box,
  Paper,
  Typography,
  FormControl,
  FormControlLabel,
  Checkbox,
  FormLabel,
  FormGroup,
} from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

const columns = [
  {
    field: "year",
    headerName: "Year",
    sortable: true,
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
  const [position, setPosition] = React.useState(["QB"]);

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
              diff: (
                player.total_actual_points - player.total_projected_points
              ).toFixed(2),
            };
          })
          .sort((a, b) => b.diff - a.diff);

        setPlayers(dataWithID);
      });
  }, [year]);

  // TODO 2024.11.06 - Remove hardcoded years and fetch all data
  const years = ["2024"];

  if (players === null) {
    return <div>Loading...</div>;
  }

  const handlePositionChange = (event) => {
    if (event.target.checked) {
      setPosition([...position, event.target.value]);
    } else {
      setPosition(position.filter((pos) => pos !== event.target.value));
    }
    // setPosition(event.target.value);
  };

  const filteredPlayers = players
    .filter((player) => {
      return position.includes(player.player_position);
    })
    .sort((a, b) => b.diff - a.diff);

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Box sx={{ marginTop: "15px" }} />
      <Typography variant="h4">Player Standings</Typography>

      <Paper
        sx={{
          padding: "10px",
        }}
      >
        <Typography variant="h6">Year:</Typography>
        <Box
          sx={{
            padding: "10px",
          }}
        >
          <FormControl component="fieldset">
            <FormGroup aria-label="year" row>
              {years.map((year) => (
                <FormControlLabel
                  key={year}
                  value={year}
                  control={<Checkbox />}
                  label={year}
                  labelPlacement="bottom"
                />
              ))}
            </FormGroup>
          </FormControl>
        </Box>
      </Paper>

      <Paper
        sx={{
          padding: "10px",
          marginTop: "15px",
        }}
      >
        <Typography variant="h6">Position:</Typography>
        <Box sx={{ padding: "10px" }}>
          <FormControl component="fieldset">
            <FormGroup aria-label="year" row>
              {["QB", "RB", "WR", "TE", "K", "DEF"].map((pos) => (
                <FormControlLabel
                  key={pos}
                  value={year}
                  control={
                    <Checkbox
                      checked={position.includes(pos)}
                      onChange={handlePositionChange}
                      value={pos}
                    />
                  }
                  label={pos}
                  labelPlacement="bottom"
                />
              ))}
            </FormGroup>
          </FormControl>
        </Box>
      </Paper>

      <Box sx={{ paddingBottom: "15px" }} />
      <DataGrid
        rows={filteredPlayers}
        columns={columns}
        rowHeight={40}
        hideFooter
      />
    </Box>
  );
};

export default Data;

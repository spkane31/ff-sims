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
  const [playersCount, setPlayersCount] = React.useState(0);
  const [years, setYear] = React.useState(["2024"]);
  const [positions, setPositions] = React.useState([
    "QB",
    "RB",
    "WR",
    "TE",
    "K",
    "D/ST",
  ]);

  const getURL = (year) => {
    if (year === "All" || year === "") {
      return `/api/boxscoreplayers`;
    }
    return `/api/boxscoreplayers?year=${year}`;
  };

  React.useEffect(() => {
    fetch(getURL(years.join(",")))
      .then((res) => res.json())
      .then((data) => {
        const dataWithID = data.data
          .map((player, index) => {
            return {
              ...player,
              id: index,
            };
          })
          .sort((a, b) => b.diff - a.diff);

        setPlayers(dataWithID);
        setPlayersCount(data.count);
      });
  }, []); // TODO seankane: add years after 2024

  if (players === null) {
    return <div>Loading...</div>;
  }

  const handlePositionChange = (event) => {
    if (event.target.checked) {
      setPositions([...positions, event.target.value]);
    } else {
      setPositions(positions.filter((pos) => pos !== event.target.value));
    }
  };

  const handleYearChange = (event) => {
    // return;
    if (event.target.checked) {
      setYear([...years, event.target.value]);
    } else {
      setYear(years.filter((yr) => yr !== event.target.value));
    }
  };

  const filteredPlayers = players
    .filter((player) => {
      return positions.includes(player.player_position);
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
      <Typography variant="h4">
        Player Standings ({playersCount} total)
      </Typography>

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
              {["2024"].map((year) => (
                <FormControlLabel
                  key={year}
                  value={year}
                  control={
                    <Checkbox
                      checked={years.includes(year)}
                      onChange={handleYearChange}
                      value="2024"
                    />
                  }
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
              {["QB", "RB", "WR", "TE", "K", "D/ST"].map((pos) => (
                <FormControlLabel
                  key={pos}
                  value={years}
                  control={
                    <Checkbox
                      checked={positions.includes(pos)}
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

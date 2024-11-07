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
    description: "The year of the player's performance",
  },
  {
    field: "player_name",
    headerName: "Player",
    sortable: true,
    width: 200,
    description: "The name of the player",
  },
  {
    field: "player_position",
    headerName: "Position",
    sortable: true,
    description: "The position the player plays",
  },
  {
    field: "total_actual_points",
    headerName: "Points",
    type: "number",
    sortable: true,
    description: "The total actual points scored by the player",
  },
  {
    field: "total_projected_points",
    headerName: "Projected Points",
    type: "number",
    sortable: true,
    description: "The total projected points for the player",
  },
  {
    field: "diff",
    headerName: "Difference",
    type: "number",
    sortable: true,
    description: "The difference between actual and projected points",
  },
  {
    field: "position_rank",
    headerName: "Position Rank",
    type: "number",
    sortable: true,
    description: "The rank of the player within their position",
  },
  {
    field: "vorp",
    headerName: "VORP",
    type: "number",
    sortable: true,
    description: "Value Over Replacement Player",
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

  const vorpByPosition = {
    QB: 20,
    RB: 20,
    WR: 20,
    TE: 10,
    K: 10,
    "D/ST": 10,
  };

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
              position_rank:
                data.data
                  .filter((p) => p.player_position === player.player_position)
                  .sort((a, b) => b.total_actual_points - a.total_actual_points)
                  .findIndex((p) => p.player_name === player.player_name) + 1,
              vorp: 0,
            };
          })
          .sort((a, b) => b.diff - a.diff);

        const vorpValue = dataWithID.reduce((acc, player) => {
          const position = player.player_position;
          if (!acc[position]) {
            acc[position] = [];
          }
          acc[position].push(player.total_actual_points);
          return acc;
        }, {});

        Object.keys(vorpValue).forEach((position) => {
          vorpValue[position].sort((a, b) => b - a);
        });

        const dataWithVORP = dataWithID
          .map((player) => {
            const position = player.player_position;
            const rank = vorpByPosition[position];
            const thresholdPoints = vorpValue[position][rank - 1] || 0;
            return {
              ...player,
              vorp: player.total_actual_points - thresholdPoints,
            };
          })
          .sort((a, b) => b.vorp - a.vorp);

        setPlayers(dataWithVORP);
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
    .sort((a, b) => b.vorp - a.vorp);

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
            marginTop: "15px",
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

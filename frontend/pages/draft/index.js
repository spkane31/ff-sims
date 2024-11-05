import React from "react";
import { Paper, Typography, Box } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";

const Draft = () => {
  const [draft, setDraft] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/draft")
      .then((res) => {
        return res.json();
      })
      .then((data) => {
        setDraft(data);
      });
  }, []);

  return draft === null ? <>Loading...</> : <DraftData draftData={draft} />;
};

const DraftData = ({ draftData }) => {
  if (draftData === null) {
    return <div>Loading...</div>;
  }

  const columns = [
    { field: "teamName", headerName: "", flex: 1, minWidth: 130 },
    { field: "playerName", headerName: "Player", flex: 1 },
    { field: "playerPosition", headerName: "Position", flex: 1 },
    { field: "roundNumber", headerName: "Round", flex: 1, type: "number" },
    { field: "pickNumber", headerName: "Pick", flex: 1, type: "number" },
    {
      field: "totalPoints",
      headerName: "Points",
      flex: 1,
      type: "number",
    },
    {
      field: "projectedPoints",
      headerName: "xPoints",
      flex: 1,
      type: "number",
    },
    {
      field: "diff",
      headerName: "Diff",
      flex: 1,
      type: "number",
    },
  ];

  const rows = Object.entries(draftData)
    .map(([_, draftSelection]) => {
      return {
        id:
          draftSelection.player_id === null
            ? Math.random()
            : draftSelection.player_id,
        teamName: draftSelection.owner,
        playerName: draftSelection.player_name,
        pickNumber: 10 * (draftSelection.round - 1) + draftSelection.pick,
        position: draftSelection.player_position,
        roundNumber: draftSelection.round,
        totalPoints: draftSelection.total_points,
        projectedPoints: draftSelection.total_projected_points,
        diff:
          draftSelection.total_points - draftSelection.total_projected_points,
      };
    })
    .sort((a, b) => a.pickNumber - b.pickNumber);

  const totalPerTeam = rows.reduce((acc, row) => {
    if (acc[row.teamName] === undefined) {
      acc[row.teamName] = {
        totalPoints: 0,
        projectedPoints: 0,
      };
    }

    acc[row.teamName].totalPoints += row.totalPoints;
    acc[row.teamName].projectedPoints += row.projectedPoints;
    return acc;
  });

  const filtered = Object.entries(totalPerTeam)
    .map(([team, stats]) => {
      if (
        stats.totalPoints === undefined ||
        stats.projectedPoints === undefined
      ) {
        return null;
      }
      return {
        id: Math.random(),
        team,
        totalPoints: stats.totalPoints,
        projectedPoints: stats.projectedPoints,
        diff: stats.totalPoints - stats.projectedPoints,
      };
    })
    .filter((team) => team !== null)
    .sort((a, b) => b.diff - a.diff);

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
        paddingTop: "25px",
        paddingBottom: "25px",
      }}
    >
      <Paper
        sx={{
          minWidth: 750,
          minHeight: 400,
        }}
      >
        <Typography
          variant="h5"
          sx={{ textAlign: "center", marginBottom: "15px" }}
        >
          Draft Results by Team
        </Typography>
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                {[
                  {
                    field: "team",
                    headerName: "",
                    flex: 1,
                  },
                  {
                    field: "totalPoints",
                    headerName: "Total Points",
                    flex: 1,
                    type: "number",
                  },
                  {
                    field: "projectedPoints",
                    headerName: "Projected Points",
                    flex: 1,
                    type: "number",
                  },
                  {
                    field: "diff",
                    headerName: "Difference",
                    flex: 1,
                    type: "number",
                  },
                ].map((column) => (
                  <TableCell key={column.field} align="right">
                    {column.headerName}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {filtered.map((row) => (
                <TableRow
                  key={row.id}
                  sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
                >
                  <TableCell component="th" scope="row">
                    {row.team}
                  </TableCell>
                  <TableCell align="right">
                    {row.totalPoints.toFixed(2)}
                  </TableCell>
                  <TableCell align="right">
                    {row.projectedPoints.toFixed(2)}
                  </TableCell>
                  <TableCell align="right">{row.diff.toFixed(2)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>

        <Typography
          variant="h5"
          sx={{ textAlign: "center", marginBottom: "15px", marginTop: "25px" }}
        >
          Draft Results
        </Typography>

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
              {rows.map((row) => (
                <TableRow
                  key={row.id}
                  sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
                >
                  <TableCell component="th" scope="row">
                    {row.team}
                  </TableCell>
                  <TableCell align="right">{row.playerName}</TableCell>
                  <TableCell align="right">{row.position}</TableCell>
                  <TableCell align="right">{row.roundNumber}</TableCell>
                  <TableCell align="right">{row.pickNumber}</TableCell>
                  <TableCell align="right">
                    {row.totalPoints.toFixed(2)}
                  </TableCell>
                  <TableCell align="right">
                    {row.projectedPoints.toFixed(2)}
                  </TableCell>
                  <TableCell align="right">{row.diff.toFixed(2)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Paper>
    </Box>
  );
};

export default Draft;

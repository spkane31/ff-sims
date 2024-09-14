import React from "react";
import { Paper, Typography, Box } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

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
    { field: "teamName", headerName: "Owner", flex: 1, minWidth: 130 },
    { field: "playerName", headerName: "Player", flex: 1 },
    { field: "roundNumber", headerName: "Round", flex: 1, type: "number" },
    { field: "pickNumber", headerName: "Pick Number", flex: 1, type: "number" },
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
    .filter((team) => team !== null);

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
        paddingTop: "25px",
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
        {/* DataGrid free version is capped at 100 rows, so either pay (not gonna happen) or write my own sortable table (doable) */}
        <DataGrid
          columns={[
            {
              field: "team",
              headerName: "Owner",
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
          ]}
          rows={filtered}
          rowHeight={30}
          autosizeOnMount
          autoHeight
          hideFooter
        />
        <Box sx={{ marginTop: "25px" }} />
        <Typography
          variant="h5"
          sx={{ textAlign: "center", marginBottom: "15px" }}
        >
          Draft Results
        </Typography>
        {/* DataGrid free version is capped at 100 rows, so either pay (not gonna happen) or write my own sortable table (doable) */}
        <DataGrid
          columns={columns}
          rows={rows}
          rowHeight={30}
          autosizeOnMount
          autoHeight
        />
      </Paper>
    </Box>
  );
};

export default Draft;

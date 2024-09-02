import React from "react";
import { Paper, Typography, Box } from "@mui/material";
import { DataGrid } from "@mui/x-data-grid";

const Draft = () => {
  const [draft, setDraft] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/draft")
      .then((res) => res.json())
      .then((data) => {
        console.log(data);
        setDraft(data);
      });
  }, []);

  return draft === null ? (
    <>Loading...</>
  ) : (
    <Box
      sx={{
        padding: "5rem 0",
        flex: 1,
        display: "flex",
        flexDirection: "column",
        justifyContent: "center",
        alignItems: "center",
        paddingLeft: "5%",
        paddingRight: "5%",
      }}
    >
      <DraftData draftData={draft} />
    </Box>
  );
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
      headerName: "Total Points",
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
        totalPoints: 0,
      };
    })
    .sort((a, b) => b.projected_wins - a.projected_wins);

  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
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

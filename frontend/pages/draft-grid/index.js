import React from "react";
import { Paper, Box, Grid } from "@mui/material";
import { styled } from "@mui/material/styles";

const Item = styled(Paper)(({ theme }) => ({
  backgroundColor: "#fff",
  ...theme.typography.body2,
  padding: theme.spacing(1),
  textAlign: "center",
  color: theme.palette.text.secondary,
  ...theme.applyStyles("dark", {
    backgroundColor: "#1A2027",
  }),
}));

const DraftGrid = () => {
  const [draft, setDraft] = React.useState(null);
  const [teams, setTeams] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/draft")
      .then((res) => res.json())
      .then((data) => {
        setDraft(data);
      });
  }, []);

  React.useEffect(() => {
    fetch("/api/teams")
      .then((res) => res.json())
      .then((data) => {
        setTeams(data);
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
      <DraftData draftData={draft} teams={teams} />
    </Box>
  );
};

const DraftData = ({ draftData, teams }) => {
  if (draftData === null || teams === null) {
    return <div>Loading...</div>;
  }

  const getTeamNameFromID = (teamID) => {
    return teams.find((team) => team.id === teamID).owner;
  };

  const rows = Object.entries(draftData)
    .map(([_, draftSelection]) => {
      return {
        id: draftSelection.player_id,
        teamName: getTeamNameFromID(draftSelection.team_id),
        playerName: draftSelection.player_name,
        pickNumber:
          10 * (draftSelection.round_number - 1) + draftSelection.round_pick,
        roundNumber: draftSelection.round_number,
        totalPoints: 0,
      };
    })
    .sort((a, b) => b.projected_wins - a.projected_wins);

  // TODO seankane: even number rounds the rows needs to be reversed because we do a snake draft
  let newRows = [];
  for (let i = 0; i < rows.length; i += 10) {
    const round = rows.slice(i, i + 10);
    if (i % 20 === 0) {
      newRows = newRows.concat(round);
    } else {
      newRows = newRows.concat(round.reverse());
    }
  }

  const headers = rows.slice(0, 10).map((row) => {
    return row.teamName;
  });

  return (
    <Grid container spacing={2}>
      <Grid item xs={12 / 11} key={"blank space"} />
      {headers.map((row) => (
        <Grid item xs={12 / 11} key={row}>
          <Item
            sx={{
              minWidth: "90px",
              maxWidth: "90px",
              minHeight: "40px",
              maxHeight: "40px",
              alignContent: "center",
              fontSize: "14px",
            }}
          >
            {row}
          </Item>
        </Grid>
      ))}
      {newRows.map((row, index) => (
        <>
          {index % 10 === 0 && (
            <Grid item xs={12 / 11} key={`Round-${index}`}>
              <Item
                sx={{
                  minWidth: "90px",
                  maxWidth: "90px",
                  minHeight: "40px",
                  maxHeight: "40px",
                  alignContent: "center",
                  fontSize: "14px",
                }}
              >
                Round {Math.ceil((index + 1) / 10)}
              </Item>
            </Grid>
          )}
          <Grid item xs={12 / 11} key={row.id}>
            <Item
              sx={{
                minWidth: "90px",
                maxWidth: "90px",
                minHeight: "40px",
                maxHeight: "40px",
                alignContent: "center",
                fontSize: "14px",
              }}
            >
              {row.playerName}
            </Item>
          </Grid>
        </>
      ))}
    </Grid>
  );
};

export default DraftGrid;

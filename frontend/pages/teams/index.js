import React from "react";
import Box from "@mui/material/Box";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Paper from "@mui/material/Paper";
import { Link } from "@mui/material";

const colors = [
  "#FF0000", // Dark red
  "#FF3333", // Dark red
  "#FF6666", // Light red
  "#FF9999", // Light red
  "#FFCCCC", // Light red
  "#FFFFFF", // White
  "#CCFFCC", // Light green
  "#99FF99", // Light green
  "#66FF66", // Light green
  "#00FF00", // Bright green
];

// Create a functional component for the page
const Data = () => {
  const [teams, setTeams] = React.useState(null);
  const [recordsGraph, setRecordsGraph] = React.useState(null);
  const [schedule, setSchedule] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/teams")
      .then((res) => res.json())
      .then((data) => {
        const filteredData = data.filter((team) => team.owner !== "League");
        setTeams(filteredData);
        filteredData.forEach((team) => {
          console.log(
            team.owner
              .split(" ")
              .filter((word) => word !== "")[1]
              .charAt(0)
              .toUpperCase() +
              team.owner
                .split(" ")
                .filter((word) => word !== "")[1]
                .slice(1)
          );
        });
      });
  }, []);

  React.useEffect(() => {
    fetch("/api/schedule?year=all")
      .then((res) => res.json())
      .then((data) => {
        setSchedule(data);
      });
  }, []);

  const recordAgainst = (teamId, opponentId) => {
    let record = { wins: 0, losses: 0 };
    schedule.forEach((week) => {
      week.forEach((game) => {
        if (
          (game.home_team_espn_id === teamId &&
            game.away_team_espn_id === opponentId) ||
          (game.home_team_espn_id === opponentId &&
            game.away_team_espn_id === teamId)
        ) {
          if (game.home_team_final_score === 0) {
            return;
          }
          if (game.home_team_final_score > game.away_team_final_score) {
            if (game.home_team_espn_id === teamId) {
              record.wins += 1;
            } else {
              record.losses += 1;
            }
          } else {
            if (game.home_team_espn_id === teamId) {
              record.losses += 1;
            } else {
              record.wins += 1;
            }
          }
        }
      });
    });
    return record;
  };

  const headers = [{ name: "", id: "" }];
  const tableData = [];
  if (teams !== null && schedule !== null) {
    teams.forEach((team) => {
      headers.push({
        name:
          team.owner
            .split(" ")
            .filter((word) => word !== "")[1]
            .charAt(0)
            .toUpperCase() +
          team.owner
            .split(" ")
            .filter((word) => word !== "")[1]
            .slice(1),
        id: team.id,
        href: `/teams/${team.id}`,
      });
    });

    teams.forEach((team) => {
      const row = [
        {
          id: `col-${team.id}`,
          field:
            team.owner
              .split(" ")
              .filter((word) => word !== "")[1]
              .charAt(0)
              .toUpperCase() +
            team.owner
              .split(" ")
              .filter((word) => word !== "")[1]
              .slice(1),
          href: `/teams/${team.id}`,
        },
      ];
      teams.forEach((opponent) => {
        if (team.id === opponent.id) {
          row.push({
            id: opponent.id,
            field: "-",
          });
          return;
        }
        const r = recordAgainst(team.id, opponent.id);
        row.push({
          id: `${team.id}-${opponent.id}`,
          field: (1 - r.wins / (r.wins + r.losses)).toFixed(3),
          wins: r.wins,
          losses: r.losses,
          blockColor:
            colors[Math.floor((1 - r.wins / (r.wins + r.losses) - 0.001) * 10)],
        });
      });
      tableData.push(row);
    });
  }

  return (
    <Box
      sx={{
        flexGrow: 1,
        padding: "15px",
        paddingTop: "15px",
        paddingBottom: "15px",
        textAlign: "center",
      }}
    >
      <Paper
        sx={{
          paddingTop: "15px",
          paddingBottom: "15px",
          textAlign: "center",
          fontSize: "20px",
        }}
      >
        Teams (winners across, losers down)
      </Paper>
      {tableData && (
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                {headers.map((team) => (
                  <TableCell key={team.id}>
                    {team.href ? (
                      <Link href={team.href}>{team.name}</Link>
                    ) : (
                      team.name.split(" ")[1]
                    )}
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {tableData.map((row) => (
                <TableRow key={row[0].id}>
                  {row.map((cell) => (
                    <TableCell
                      key={cell.id}
                      sx={{
                        backgroundColor: cell.blockColor
                          ? cell.blockColor
                          : "inherit",
                      }}
                    >
                      {cell.href ? (
                        <Link href={cell.href}>
                          {cell.field ? cell.field : ""}
                        </Link>
                      ) : (
                        cell.field
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
};

export default Data;

import React from "react";
import Box from "@mui/material/Box";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Paper from "@mui/material/Paper";

// Create a functional component for the page
const Data = () => {
  const [teams, setTeams] = React.useState(null);
  const [recordsGraph, setRecordsGraph] = React.useState(null);

  React.useEffect(() => {
    fetch("/api/teams")
      .then((res) => res.json())
      .then((data) => {
        const filteredData = data.filter((team) => team.owner !== "League");
        setTeams(filteredData);
      });
  }, []);

  React.useEffect(() => {
    fetch("/api/schedule?year=all")
      .then((res) => res.json())
      .then((data) => {
        let recordsGraph = {};

        data.forEach((week) => {
          week.forEach((game) => {
            const key = `${game.home_team_espn_id}-${game.away_team_espn_id}`;
            if (!recordsGraph[key]) {
              recordsGraph[key] = {
                home: {
                  wins: 0,
                  losses: 0,
                },
                away: {
                  wins: 0,
                  losses: 0,
                },
              };
            }

            if (game.home_team_final_score > game.away_team_final_score) {
              recordsGraph[key].home.wins += 1;
              recordsGraph[key].away.losses += 1;
            } else {
              recordsGraph[key].home.losses += 1;
              recordsGraph[key].away.wins += 1;
            }
          });
        });

        console.log(recordsGraph);

        setRecordsGraph(recordsGraph);
      });
  }, []);

  const tableData = [];
  if (teams !== null && recordsGraph !== null) {
    const headers = [{ name: "", id: "" }];
    teams.forEach((team) => {
      headers.push({
        name: team.owner,
        id: team.id,
      });
    });
    tableData.push(headers);

    headers.slice(1).map((team) => {
      const rows = headers.slice(1).map((opponent) => {
        if (team.id === opponent.id) {
          return { id: team.id, name: "-" };
        }

        const key = `${team.id}-${opponent.id}`;
        const record = recordsGraph[key];

        if (record) {
          if (team.id === parseInt(key.split("-")[0])) {
            return { field: `${record.home.wins}-${record.home.losses}` };
          } else {
            return { field: `${record.away.wins}-${record.away.losses}` };
          }
        } else {
          return { field: "X" };
        }
      });
      tableData.push([{ id: team.id, name: "" }, ...rows]);
    });

    // tableData.push([teams[tableData.length - 1], ...rows]);
  } else {
    return <div>Loading...</div>;
  }

  console.log("tableData", tableData);

  return (
    <Box sx={{ flexGrow: 1, padding: "15px", textAlign: "center" }}>
      Teams
      {tableData && (
        <TableContainer component={Paper}>
          <Table>
            <TableHead>
              <TableRow>
                {tableData[0].map((team) => (
                  <TableCell key={team.id}>{team.name}</TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {tableData.slice(1).map((row) => (
                <TableRow key={row.id}>
                  {row.map((cell) => (
                    <TableCell key={cell.field}></TableCell>
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

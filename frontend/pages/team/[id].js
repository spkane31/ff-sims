import React from "react";
import Head from "next/head";
import { useRouter } from "next/router";
import {
  Box,
  Paper,
  Typography,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
} from "@mui/material";

import TitleComponent from "../../components/TitleComponent";

const paddingAmount = "15px";

export default function Team() {
  const router = useRouter();
  const [data, setData] = React.useState([]);
  const [schedule, setSchedule] = React.useState([]);
  const { id } = router.query;

  React.useEffect(() => {
    if (!id) {
      return;
    }
    async function fetchData() {
      const response = await fetch("/api/team/" + router.query.id);
      const data = await response.json();
      setData(data);
      console.log(data);
    }

    fetchData();
  }, [id]);

  React.useEffect(() => {
    if (!id) {
      return;
    }
    async function fetchData() {
      const response = await fetch("/api/schedule/" + router.query.id);
      const data = await response.json();
      setSchedule(data);
      console.log(data);
    }

    fetchData();
  }, [id]);

  if (data.length === 0) {
    return <></>;
  }

  return (
    <>
      <Box
        sx={{
          padding: paddingAmount,
        }}
      />
      <Head>The League FF</Head>
      <TitleComponent>{data.owner}</TitleComponent>
      <Box sx={{ padding: paddingAmount }} />
      <Historical data={data} />
      <Box sx={{ padding: paddingAmount }} />
      <ByTeam data={data.opponents} />
      <Box sx={{ padding: paddingAmount }} />
      <Schedule schedule={schedule} />
      <Box sx={{ padding: paddingAmount }} />
    </>
  );
}

const Historical = ({ data }) => {
  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 500,
        }}
      >
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Wins</TableCell>
              <TableCell>Losses</TableCell>
              <TableCell>Points For</TableCell>
              <TableCell>Points Against</TableCell>
              <TableCell>Record</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <TableRow>
              <TableCell>{data.historical.wins}</TableCell>
              <TableCell>{data.historical.losses}</TableCell>
              <TableCell>{data.historical.points.toLocaleString()}</TableCell>
              <TableCell>
                {data.historical.opp_points.toLocaleString()}
              </TableCell>
              <TableCell>
                {data.historical.wins === 0 && data.historical.losses === 0
                  ? 0.0
                  : (
                      data.historical.wins /
                      (data.historical.wins + data.historical.losses)
                    ).toFixed(3)}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </Paper>
    </Box>
  );
};

const Schedule = ({ schedule }) => {
  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 500,
          minHeight: 400,
        }}
      >
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Year</TableCell>
              <TableCell>Week</TableCell>
              <TableCell>Team</TableCell>
              <TableCell>Team Score</TableCell>
              <TableCell>Opponent Score</TableCell>
              <TableCell>Opponent</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {schedule.map((game) => (
              <TableRow
                key={game.id}
                sx={{
                  backgroundColor:
                    game.team_score > game.opponent_score
                      ? "lightgreen"
                      : "red",
                }}
              >
                <TableCell>{game.year}</TableCell>
                <TableCell>{game.week}</TableCell>
                <TableCell>{game.owner}</TableCell>
                <TableCell>{game.team_score}</TableCell>
                <TableCell>{game.opponent_score}</TableCell>
                <TableCell>{game.opponent_owner}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>
    </Box>
  );
};

const ByTeam = ({ data }) => {
  return (
    <Box
      sx={{
        maxWidth: "100%",
        overflowX: "auto",
      }}
    >
      <Paper
        sx={{
          minWidth: 500,
          minHeight: 400,
        }}
      >
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Opponent</TableCell>
              <TableCell>Wins</TableCell>
              <TableCell>Losses</TableCell>
              <TableCell>Team Score</TableCell>
              <TableCell>Opponent Score</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {data.map((opponent) => (
              <TableRow
                key={opponent.opponent_id}
                sx={{
                  backgroundColor:
                    opponent.wins > opponent.losses ? "lightgreen" : "red",
                }}
              >
                <TableCell>{opponent.opponent_owner}</TableCell>
                <TableCell>{opponent.wins}</TableCell>
                <TableCell>{opponent.losses}</TableCell>
                <TableCell>{opponent.team_score}</TableCell>
                <TableCell>{opponent.opponent_score}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>
    </Box>
  );
};

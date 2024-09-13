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
  Link,
} from "@mui/material";

import TitleComponent from "../../components/TitleComponent";

const paddingAmount = "15px";

export default function Team() {
  const router = useRouter();
  const [data, setData] = React.useState([]);
  const [schedule, setSchedule] = React.useState([]);
  const [groupedByTeam, setGroupedByTeam] = React.useState([]);
  const { id } = router.query;

  React.useEffect(() => {
    if (!id) {
      return;
    }
    async function fetchData() {
      const response = await fetch("/api/team/" + router.query.id);
      const data = await response.json();
      setData(data);
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
      convertScheduleToGroup(data);
    }

    fetchData();
  }, [id]);

  const convertScheduleToGroup = (schedule) => {
    const groupedByTeam = new Map();
    schedule.forEach((match) => {
      if (!groupedByTeam.has(match.opponent_id)) {
        groupedByTeam.set(match.opponent_id, {
          opponent_id: match.opponent_id,
          opponent_owner: match.opponent_owner,
          team_score: match.team_score,
          opponent_score: match.opponent_score,
          wins: match.team_score > match.opponent_score ? 1 : 0,
          losses: match.team_score < match.opponent_score ? 1 : 0,
          draws: match.team_score === match.opponent_score ? 1 : 0,
        });
      } else {
        groupedByTeam.get(match.opponent_id).team_score += match.team_score;
        groupedByTeam.get(match.opponent_id).opponent_score +=
          match.opponent_score;
        groupedByTeam.get(match.opponent_id).wins +=
          match.team_score > match.opponent_score ? 1 : 0;
        groupedByTeam.get(match.opponent_id).losses +=
          match.team_score < match.opponent_score ? 1 : 0;
        groupedByTeam.get(match.opponent_id).draws +=
          match.team_score === match.opponent_score ? 1 : 0;
      }
    });

    setGroupedByTeam(
      Array.from(groupedByTeam.values())
        .filter((row) => {
          return row.opponent_id !== 2 && row.opponent_id !== 8;
        })
        .sort((a, b) => {
          return (
            b.team_score - b.opponent_score - (a.team_score - a.opponent_score)
          );
        })
    );
  };

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
      <ByTeam data={groupedByTeam} />
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
              <TableCell>Differential</TableCell>
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
                {(
                  data.historical.points - data.historical.opp_points
                ).toLocaleString()}
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
                      : "lightcoral",
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
              <TableCell>Diff</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {data.map((opponent) => (
              <TableRow
                key={opponent.opponent_id}
                sx={{
                  backgroundColor:
                    opponent.wins > opponent.losses
                      ? "lightgreen"
                      : opponent.wins < opponent.losses
                      ? "lightcoral"
                      : "",
                }}
              >
                <TableCell>
                  <Link href={`/team/${opponent.opponent_id}`}>
                    {opponent.opponent_owner}
                  </Link>
                </TableCell>
                <TableCell>{opponent.wins}</TableCell>
                <TableCell>{opponent.losses}</TableCell>
                <TableCell>
                  {opponent.team_score.toFixed(2).toLocaleString()}
                </TableCell>
                <TableCell>
                  {opponent.opponent_score.toFixed(2).toLocaleString()}
                </TableCell>
                <TableCell>
                  {(opponent.team_score - opponent.opponent_score)
                    .toFixed(2)
                    .toLocaleString()}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>
    </Box>
  );
};

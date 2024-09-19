// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { logRequest, pool } from "../../db/db";

const query = `
SELECT
  teams.espn_id AS id,
  teams.owner AS owner,
  week,
  year
FROM (
  SELECT home_team_espn_id AS team_espn_id, year, week
  FROM matchups
  WHERE year = $1 AND completed = true
  UNION ALL
  SELECT away_team_espn_id AS team_espn_id, year, week
  FROM matchups
  WHERE year = $1 AND completed = true
) AS scores
JOIN teams
ON scores.team_espn_id = teams.espn_id
`;

export default async function perfectrosters(req, res) {
  const start = new Date();
  try {
    const client = await pool.connect();
    const resp = await client.query(query, [2024]);

    const perfectrosters = [];

    for (const roster of resp.rows) {
      const weekResp = await client.query(
        `
      SELECT
        player_name,
        player_position,
        actual_points,
        status
      FROM box_score_players
      WHERE week = $1 AND year = $2 AND owner_espn_id = $3
      `,
        [roster.week, roster.year, roster.id]
      );

      if (
        isPerfectRoster(
          weekResp.rows
            .sort((a, b) => b.actual_points - a.actual_points)
            .map((player) => ({
              player_name: player.player_name,
              player_position: player.player_position,
              actual_points: parseFloat(player.actual_points),
              status: player.status,
            }))
        )
      ) {
        perfectrosters.push({
          owner: roster.owner,
          week: roster.week,
          year: roster.year,
        });
      }
    }

    client.release();

    res.status(200).json(perfectrosters);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
  logRequest(req, res, start);
}

function isPerfectRoster(roster) {
  const totalPoints = roster
    .filter((player) => player.status !== "BE")
    .reduce((acc, player) => acc + player.actual_points, 0);

  return perfectRosterPoints(roster) === totalPoints;
}

export function perfectRosterPoints(roster) {
  // find the top two qbs
  let qbs = roster
    .filter((player) => player.player_position === "QB")
    .slice(0, 2);
  let rbs = roster
    .filter(
      (player) =>
        player.player_position === "RB" || player.player_position === "RB/WR/TE"
    )
    .slice(0, 2);
  let wrs = roster
    .filter(
      (player) =>
        player.player_position === "WR" || player.player_position === "RB/WR/TE"
    )
    .slice(0, 2);
  let tes = roster
    .filter(
      (player) =>
        player.player_position === "TE" || player.player_position === "RB/WR/TE"
    )
    .slice(0, 1);
  let dst = roster
    .filter((player) => player.player_position === "D/ST")
    .slice(0, 1);
  let ks = roster
    .filter((player) => player.player_position === "K")
    .slice(0, 1);

  const perfectRoster = [...qbs, ...rbs, ...wrs, ...tes, ...dst, ...ks];

  // find the next highest scoring RB/WR/TE
  const flex = roster
    .filter(
      (player) =>
        (player.player_position === "RB" ||
          player.player_position === "WR" ||
          player.player_position === "TE") &&
        !perfectRoster.includes(player)
    )
    .slice(0, 1);

  // add up the total points of the perfect roster
  const perfectPoints =
    qbs.reduce((acc, player) => acc + player.actual_points, 0) +
    rbs.reduce((acc, player) => acc + player.actual_points, 0) +
    wrs.reduce((acc, player) => acc + player.actual_points, 0) +
    tes.reduce((acc, player) => acc + player.actual_points, 0) +
    dst.reduce((acc, player) => acc + player.actual_points, 0) +
    ks.reduce((acc, player) => acc + player.actual_points, 0) +
    flex.reduce((acc, player) => acc + player.actual_points, 0);

  return perfectPoints;
}

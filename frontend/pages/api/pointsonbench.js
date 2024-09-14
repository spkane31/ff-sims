// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../db/db";
import { perfectRosterPoints } from "./perfectrosters";

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
  try {
    const client = await pool.connect();
    const resp = await client.query(query, [2024]);

    const missingPoints = new Map();

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

      const parsed = weekResp.rows
        .sort((a, b) => b.actual_points - a.actual_points)
        .map((player) => ({
          player_name: player.player_name,
          player_position: player.player_position,
          actual_points: parseFloat(player.actual_points),
          status: player.status,
        }));

      const totalPoints = parsed
        .filter((player) => player.status !== "BE")
        .reduce((acc, player) => acc + player.actual_points, 0);

      const perfectPoints = perfectRosterPoints(parsed);

      // add the diff to the missingPoints map with key of owner
      if (missingPoints.get(roster.owner) === undefined) {
        missingPoints.set(roster.owner, 0);
      }

      missingPoints.set(
        roster.owner,
        missingPoints.get(roster.owner) + perfectPoints - totalPoints
      );
    }

    client.release();

    const ret = [];
    missingPoints.forEach((value, key) => {
      ret.push({
        owner: key,
        missingPoints: value,
      });
    });

    res.status(200).json(ret.sort((a, b) => b.missingPoints - a.missingPoints));
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

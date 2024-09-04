// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../db/db";

const query = `
SELECT
  year,
  player_id,
  player_name,
  player_position,
  sum(projected_points) AS total_projected_points,
  sum(actual_points) AS total_actual_points
FROM box_score_players
WHERE year = $1
GROUP BY player_id, year, player_name, player_position
ORDER BY total_actual_points DESC
LIMIT 50;
`;

const queryAll = `
SELECT
  year,
  player_id,
  player_name,
  player_position,
  sum(projected_points) AS total_projected_points,
  sum(actual_points) AS total_actual_points
FROM box_score_players
GROUP BY player_id, year, player_name, player_position
ORDER BY total_actual_points DESC
LIMIT 50;
`;

async function runQuery(year) {
  const client = await pool.connect();
  if (year === undefined) {
    const resp = await client.query(queryAll);
    client.end();
    return resp;
  }
  const resp = await client.query(query, [year]);
  client.end();
  return resp;
}

export default async function draft(req, res) {
  try {
    // get year from query param and default to current year if not present
    const resp = await runQuery(req.query.year);

    const ret = resp.rows.map((row) => {
      return {
        year: parseInt(row.year),
        player_id: parseInt(row.player_id),
        player_name: row.player_name,
        player_position: row.player_position,
        total_projected_points: parseFloat(row.total_projected_points),
        total_actual_points: parseFloat(row.total_actual_points),
      };
    });

    res.status(200).json(ret);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

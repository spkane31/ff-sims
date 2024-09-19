// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { logRequest, pool } from "../../db/db";

const query = `
SELECT
  round,
  pick,
  player_name,
  player_id,
  draft_selections.owner_espn_id AS team_id,
  teams.owner AS owner
FROM draft_selections
JOIN teams
ON draft_selections.owner_espn_id = teams.espn_id
WHERE year = $1
ORDER BY round, pick;
`;

const totalPointsPerPlayer = `
SELECT
  player_id,
  SUM(projected_points) AS total_projected_points,
  SUM(actual_points) AS total_points,
  COUNT(*) AS num_games
FROM box_score_players
WHERE year = $1
GROUP BY player_id;
`;

export default async function draft(req, res) {
  const start = new Date();
  try {
    const client = await pool.connect();
    const resp = await client.query(query, [2024]);
    console.log(
      `[INFO] received ${resp.rows.length} rows from draft 2024 query`
    );
    const playerPoints = await client.query(totalPointsPerPlayer, [2024]);
    client.release();

    const parsedResp = resp.rows.map((row) => {
      return {
        round: parseInt(row.round),
        pick: parseInt(row.pick),
        player_name: row.player_name,
        player_id: parseInt(row.player_id),
        team_id: parseInt(row.team_id),
        total_points: parseFloat(
          playerPoints.rows.find(
            (player) => parseInt(player.player_id) === parseInt(row.player_id)
          )?.total_points || 0
        ),
        total_projected_points: parseFloat(
          playerPoints.rows.find(
            (player) => parseInt(player.player_id) === parseInt(row.player_id)
          )?.total_projected_points || 0
        ),
        count: parseInt(
          playerPoints.rows.find(
            (player) => parseInt(player.player_id) === parseInt(row.player_id)
          )?.num_games || 0
        ),
        owner: row.owner,
      };
    });

    res.status(200).json(parsedResp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
  logRequest(req, res, start);
}

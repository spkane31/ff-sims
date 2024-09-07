// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../db/db";

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

export default async function draft(req, res) {
  try {
    const client = await pool.connect();
    const resp = await client.query(query, [2024]);
    client.end();

    const parsedResp = resp.rows.map((row) => {
      return {
        round: parseInt(row.round),
        pick: parseInt(row.pick),
        player_name: row.player_name,
        player_id: parseInt(row.player_id),
        team_id: parseInt(row.team_id),
        owner: row.owner,
      };
    });

    res.status(200).json(parsedResp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

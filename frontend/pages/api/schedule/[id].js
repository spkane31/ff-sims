// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../../db/db";

const query = `
SELECT
  CASE
    WHEN home_team_espn_id = $1 THEN away_team_espn_id
    ELSE home_team_espn_id
  END AS opponent,
  CASE
    WHEN home_team_espn_id = $1 THEN home_team_final_score
    ELSE away_team_final_score
  END AS team_score,
  CASE
    WHEN home_team_espn_id = $1 THEN away_team_final_score
    ELSE home_team_final_score
  END AS opponent_score,
  week,
  year,
  id
FROM matchups
WHERE home_team_espn_id = $1 OR away_team_espn_id = $1
ORDER BY year desc, week desc;
`;

export default async function schedule(req, res) {
  try {
    // Get ID from path
    const id = req.query.id;

    const client = await pool.connect();
    const resp = await client.query(query, [id]);
    console.log(
      `[INFO] received ${resp.rows.length} rows from schedule ${id} query`
    );

    const teams = await client.query(`SELECT espn_id, owner FROM teams;`);
    console.log(
      `[INFO] received ${teams.rows.length} rows from schedule query`
    );
    client.end();

    const parsedResp = resp.rows.map((row) => {
      return {
        team_id: parseInt(id),
        owner: teams.rows.find((team) => team.espn_id === id).owner,
        team_score: parseFloat(row.team_score),
        opponent: parseInt(row.opponent),
        opponent_score: parseFloat(row.opponent_score),
        opponent_owner: teams.rows.find((team) => team.espn_id === row.opponent)
          .owner,
        opponent_id: parseInt(row.opponent),
        week: parseInt(row.week),
        year: parseInt(row.year),
      };
    });

    res.status(200).json(parsedResp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

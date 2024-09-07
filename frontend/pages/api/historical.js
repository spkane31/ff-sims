// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../db/db";

const query = `
SELECT team_id,
       SUM(wins) AS wins,
       SUM(losses) AS losses,
       SUM(draws) AS draws,
       SUM(points) AS points
FROM (
  SELECT home_team_espn_id AS team_id,
         COUNT(*) FILTER (WHERE home_team_final_score > away_team_final_score) AS wins,
         COUNT(*) FILTER (WHERE home_team_final_score < away_team_final_score) AS losses,
         COUNT(*) FILTER (WHERE home_team_final_score = away_team_final_score) AS draws,
         SUM(home_team_final_score) AS points
  FROM matchups
  WHERE home_team_final_score > 0 AND away_team_final_score > 0
  GROUP BY home_team_espn_id
  UNION ALL
  SELECT away_team_espn_id AS team_id,
         COUNT(*) FILTER (WHERE away_team_final_score > home_team_final_score) AS wins,
         COUNT(*) FILTER (WHERE away_team_final_score < home_team_final_score) AS losses,
         COUNT(*) FILTER (WHERE away_team_final_score = home_team_final_score) AS draws,
         SUM(away_team_final_score) AS points
  FROM matchups
  WHERE home_team_final_score > 0 AND away_team_final_score > 0
  GROUP BY away_team_espn_id
) AS records
GROUP BY team_id;
`;

`SELECT espn_id, owner FROM teams;`;

export default async function historical(req, res) {
  try {
    const client = await pool.connect();
    const resp = await client.query(query);

    const teams = await client.query(`SELECT espn_id, owner FROM teams;`);
    client.end();

    const parsedResp = resp.rows.map((row) => {
      return {
        team_id: parseInt(row.team_id),
        owner: teams.rows.find((team) => team.espn_id === row.team_id).owner,
        wins: parseInt(row.wins),
        losses: parseInt(row.losses),
        draws: parseInt(row.draws),
        points: parseFloat(row.points),
      };
    });

    res.status(200).json(parsedResp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

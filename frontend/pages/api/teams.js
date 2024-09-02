// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { Pool } from "pg/lib";
import { config } from "../../config";
const pool = new Pool(config);

const query = `SELECT
  teams.espn_id AS id,
  teams.owner AS owner,
  AVG(scores.score) AS average_score,
  STDDEV(scores.score) AS stddev_score
FROM (
  SELECT home_team_espn_id AS team_espn_id, home_team_final_score AS score
  FROM matchups
  WHERE year = $1
  UNION ALL
  SELECT away_team_espn_id AS team_espn_id, away_team_final_score AS score
  FROM matchups
  WHERE year = $1
) AS scores
JOIN teams
ON scores.team_espn_id = teams.espn_id
GROUP BY teams.espn_id, teams.owner;`;

const queryLeague = `SELECT
  AVG(scores.score) AS average_score,
  STDDEV(scores.score) AS stddev_score
FROM (
  SELECT home_team_final_score AS score
  FROM matchups
  WHERE year = $1
  UNION ALL
  SELECT away_team_final_score AS score
  FROM matchups
  WHERE year = $1
) AS scores;`;

export default async function teams(req, res) {
  try {
    const client = await pool.connect();
    const teamData = await client.query(query, [2024]);
    const respIDAsInt = teamData.rows.map((row) => {
      return {
        id: parseInt(row.id),
        owner: row.owner,
        averageScore:
          row.average_score === null
            ? 0.0
            : parseFloat(row.average_score).toFixed(3),
        stddevScore:
          row.stddev_score === null
            ? 0.0
            : parseFloat(row.stddev_score).toFixed(3),
      };
    });

    const leagueData = await client.query(queryLeague, [2024]);

    respIDAsInt.push({
      id: -1,
      owner: "League",
      averageScore:
        leagueData.rows[0].average_score === null
          ? 0.0
          : parseFloat(leagueData.rows[0].average_score).toFixed(3),
      stddevScore:
        leagueData.rows[0].stddev_score === null
          ? 0.0
          : parseFloat(leagueData.rows[0].stddev_score).toFixed(3),
    });

    res.status(200).json(respIDAsInt);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

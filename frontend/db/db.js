import { Pool } from "pg/lib";
import { config } from "../config";
export const pool = new Pool(config);

export const getTeams = async (year) => {
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

  const client = await pool.connect();
  const teamData = await client.query(query, [year]);
  const respIDAsInt = teamData.rows.map((row) => {
    return {
      id: parseInt(row.id),
      owner: row.owner,
      averageScore:
        row.average_score === null ? 0.0 : parseFloat(row.average_score),
      stddevScore:
        row.stddev_score === null ? 0.0 : parseFloat(row.stddev_score),
    };
  });

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

  const leagueData = await client.query(queryLeague, [year]);

  respIDAsInt.push({
    id: -1,
    owner: "League",
    averageScore:
      leagueData.rows[0].average_score === null
        ? 0.0
        : parseFloat(leagueData.rows[0].average_score),
    stddevScore:
      leagueData.rows[0].stddev_score === null
        ? 0.0
        : parseFloat(leagueData.rows[0].stddev_score),
  });

  client.end();

  return respIDAsInt;
};

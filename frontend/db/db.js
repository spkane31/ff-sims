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
  console.log(`[INFO] received ${teamData.rows.length} rows from teams query`);
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
  console.log(
    `[INFO] received ${leagueData.rows.length} rows from league data query`
  );
  client.end();

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

  return respIDAsInt;
};

const scheduleQuery = `
SELECT
  matchups.year AS year,
  matchups.week AS week,
  matchups.home_team_espn_id,
  matchups.away_team_espn_id,
  t.owner AS home_team_owner,
  t2.owner AS away_team_owner,
  matchups.completed,
  matchups.home_team_final_score AS home_team_final_score,
  matchups.away_team_final_score AS away_team_final_score
FROM matchups
JOIN teams AS t ON matchups.home_team_espn_id = t.espn_id
JOIN teams AS t2 ON matchups.away_team_espn_id = t2.espn_id
WHERE year = 2024
ORDER BY week;`;

export const getSchedule = async (year) => {
  const client = await pool.connect();
  const resp = await client.query(scheduleQuery);
  console.log(`[INFO] received ${resp.rows.length} rows from schedule query`);
  client.end();

  const parsedResponse = resp.rows.map((row) => {
    return {
      year: parseInt(row.year),
      week: parseInt(row.week),
      home_team_espn_id: parseInt(row.home_team_espn_id),
      away_team_espn_id: parseInt(row.away_team_espn_id),
      home_team_owner: row.home_team_owner,
      away_team_owner: row.away_team_owner,
      home_team_final_score:
        row.home_team_final_score === null ? 0.0 : row.home_team_final_score,
      away_team_final_score:
        row.away_team_final_score === null ? 0.0 : row.away_team_final_score,
      completed: row.completed === null ? false : row.completed,
    };
  });

  // Want to return a list of lists where each week is grouped into its own list
  const schedule = [];
  let currentWeek = 1;
  let currentWeekGames = [];
  parsedResponse.forEach((game) => {
    if (game.week === currentWeek) {
      currentWeekGames.push(game);
    } else {
      schedule.push(currentWeekGames);
      currentWeekGames = [game];
      currentWeek++;
    }
  });
  schedule.push(currentWeekGames);
  return schedule;
};

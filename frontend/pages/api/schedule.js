// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { Pool } from "pg/lib";
import { config } from "../../config";
const pool = new Pool(config);

const query = `
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

export default async function hello(req, res) {
  try {
    const client = await pool.connect();
    const resp = await client.query(query);
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

    console.log(schedule);

    res.status(200).json(schedule);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

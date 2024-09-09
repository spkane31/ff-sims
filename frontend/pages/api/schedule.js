// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../db/db";

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
  matchups.away_team_final_score AS away_team_final_score,
  matchups.home_team_espn_projected_score AS home_team_espn_projected_score,
  matchups.away_team_espn_projected_score AS away_team_espn_projected_score
FROM matchups
JOIN teams AS t ON matchups.home_team_espn_id = t.espn_id
JOIN teams AS t2 ON matchups.away_team_espn_id = t2.espn_id
WHERE year = 2024
ORDER BY week;`;

export default async function schedule(req, res) {
  try {
    const client = await pool.connect();
    const resp = await client.query(query);
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
          row.home_team_final_score === null
            ? 0.0
            : parseFloat(row.home_team_final_score),
        away_team_final_score:
          row.away_team_final_score === null
            ? 0.0
            : parseFloat(row.away_team_final_score),
        completed: row.completed === null ? false : row.completed,
        home_team_espn_projected_score: parseFloat(
          row.home_team_espn_projected_score
        ),
        away_team_espn_projected_score: parseFloat(
          row.away_team_espn_projected_score
        ),
      };
    });

    console.log(
      `[INFO] parsed ${parsedResponse.length} rows from parsedResponse map func`
    );

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

    console.log(
      `[INFO] parsed ${schedule.length} week(s) from schedule map func`
    );

    res.status(200).json(schedule);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

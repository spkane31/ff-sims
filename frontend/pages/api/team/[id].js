// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { pool } from "../../../db/db";

const query = `
SELECT team_id,
       SUM(wins) AS wins,
       SUM(losses) AS losses,
       SUM(draws) AS draws,
       SUM(points) AS points,
       SUM(opp_points) AS opp_points
FROM (
  SELECT home_team_espn_id AS team_id,
         COUNT(*) FILTER (WHERE home_team_final_score > away_team_final_score) AS wins,
         COUNT(*) FILTER (WHERE home_team_final_score < away_team_final_score) AS losses,
         COUNT(*) FILTER (WHERE home_team_final_score = away_team_final_score) AS draws,
         SUM(home_team_final_score) AS points,
         SUM(away_team_final_score) AS opp_points
  FROM matchups
  WHERE home_team_espn_id = $1
  GROUP BY home_team_espn_id
  UNION ALL
  SELECT away_team_espn_id AS team_id,
         COUNT(*) FILTER (WHERE away_team_final_score > home_team_final_score) AS wins,
         COUNT(*) FILTER (WHERE away_team_final_score < home_team_final_score) AS losses,
         COUNT(*) FILTER (WHERE away_team_final_score = home_team_final_score) AS draws,
         SUM(away_team_final_score) AS points,
         SUM(home_team_final_score) AS opp_points
  FROM matchups
  WHERE away_team_espn_id = $1
  GROUP BY away_team_espn_id
) AS records
GROUP BY team_id;
`;

const opponentQuery = `
SELECT
  *
FROM (
  SELECT
    home_team_espn_id AS team_id,
    away_team_espn_id AS opponent_id,
    home_team_final_score AS team_score,
    away_team_final_score AS opponent_score
  FROM matchups
  WHERE home_team_espn_id = $1
  UNION ALL
  SELECT
    home_team_espn_id AS opponent_id,
    away_team_espn_id AS team_id,
    home_team_final_score AS opponent_score,
    away_team_final_score AS team_score
  FROM matchups
  WHERE away_team_espn_id = $1
) AS records;
`;

export default async function schedule(req, res) {
  try {
    // Get ID from path
    const id = req.query.id;

    const client = await pool.connect();
    const resp = await client.query(query, [id]);
    const parsedResp = resp.rows.map((row) => {
      return {
        id: parseInt(row.team_id),
        wins: parseInt(row.wins),
        losses: parseInt(row.losses),
        draws: parseInt(row.draws),
        points: parseFloat(row.points),
        opp_points: parseFloat(row.opp_points),
      };
    });

    const teams = await client.query(`SELECT espn_id, owner FROM teams;`);
    const parsedTeams = teams.rows.map((row) => {
      return {
        id: parseInt(row.espn_id),
        owner: row.owner,
      };
    });

    console.log("teams: ", parsedTeams);

    const opponentResp = await client.query(opponentQuery, [id]);

    client.end();

    const opponents = opponentResp.rows.map((row) => {
      return {
        team_id: parseInt(row.team_id),
        owner: parsedTeams.find((team) => team.id === parseInt(row.team_id))
          .owner,
        opponent_id: parseInt(row.opponent_id),
        opponent_owner: parsedTeams.find(
          (team) => team.id === parseInt(row.opponent_id)
        ).owner,
        team_score: parseFloat(row.team_score),
        opponent_score: parseFloat(row.opponent_score),
      };
    });

    console.log("OPPONENTS: ", opponents);

    const groupedByTeam = new Map();
    opponents.forEach((match) => {
      if (!groupedByTeam.has(match.opponent_id)) {
        groupedByTeam.set(match.opponent_id, {
          opponent_id: match.opponent_id,
          opponent_owner: match.opponent_owner,
          team_score: match.team_score,
          opponent_score: match.opponent_score,
          wins: match.team_score > match.opponent_score ? 1 : 0,
          losses: match.team_score < match.opponent_score ? 1 : 0,
          draws: match.team_score === match.opponent_score ? 1 : 0,
        });
      } else {
        groupedByTeam.get(match.opponent_id).team_score += match.team_score;
        groupedByTeam.get(match.opponent_id).opponent_score +=
          match.opponent_score;
        groupedByTeam.get(match.opponent_id).wins +=
          match.team_score > match.opponent_score ? 1 : 0;
        groupedByTeam.get(match.opponent_id).losses +=
          match.team_score < match.opponent_score ? 1 : 0;
        groupedByTeam.get(match.opponent_id).draws +=
          match.team_score === match.opponent_score ? 1 : 0;
      }
    });

    console.log("GROUPED BY TEAM: ", groupedByTeam);

    const opponentsArr = Array.from(groupedByTeam.values())
      .sort((a, b) => {
        return b.wins - b.losses - (a.wins - a.losses);
      })
      .filter(
        (opponent) =>
          opponent.opponent_id !== 8 &&
          opponent.opponent_id !== 2 &&
          opponent.opponent_id !== id
      );

    res.status(200).json({
      historical: parsedResp[0],
      owner: parsedTeams.find((team) => team.id === parseInt(id)).owner,
      opponents: opponentsArr,
    });
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

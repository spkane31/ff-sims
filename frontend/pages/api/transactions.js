// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { logRequest, pool } from "../../db/db";

const query = `
SELECT DISTINCT
  t.date AS date,
  t.id AS id,
  t.transaction_type AS transaction_type,
  teams.owner AS owner,
  players.player_name AS player_name
FROM transactions t
JOIN teams ON t.team_id = teams.espn_id
JOIN box_score_players AS players ON t.player_id = players.player_id
ORDER BY t.date DESC
LIMIT 1000;
`;

export default async function transactions(req, res) {
  const start = new Date();
  try {
    const client = await pool.connect();
    const resp = await client.query(query);

    let txs = {};

    resp.rows.forEach((tx) => {
      if (txs[tx.date] === undefined) {
        txs[tx.date] = {
          date: tx.date,
          transactions: [
            {
              transaction_type: tx.transaction_type,
              owner: tx.owner,
              player_name: tx.player_name,
            },
          ],
        };
      } else {
        txs[tx.date].transactions.push({
          transaction_type: tx.transaction_type,
          owner: tx.owner,
          player_name: tx.player_name,
        });
      }
    });

    let finalResp = [];
    for (const key in txs) {
      finalResp.push(txs[key]);
    }

    finalResp = finalResp.flatMap((tx) => {
      const hasTraded = tx.transactions.some(
        (t) => t.transaction_type === "TRADED"
      );
      if (!hasTraded) {
        const groupedByOwner = tx.transactions.reduce((acc, t) => {
          if (!acc[t.owner]) {
            acc[t.owner] = [];
          }
          acc[t.owner].push(t);
          return acc;
        }, {});

        return Object.keys(groupedByOwner).map((owner) => ({
          date: tx.date,
          transactions: groupedByOwner[owner],
        }));
      }
      return [tx];
    });

    res.status(200).json(finalResp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
  logRequest(req, res, start);
}

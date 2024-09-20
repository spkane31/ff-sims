// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { logRequest, pool } from "../../db/db";

export default async function webanalytics(req, res) {
  const start = new Date();
  const year = 2024;
  try {
    const client = await pool.connect();

    const query = `SELECT
      id, endpoint, method, body, user_agent, is_frontend, timestamp
    FROM requests;`;

    const allRequests = await client.query(query);
    console.log(
      `[INFO] received ${allRequests.rows.length} rows from allRequests query`
    );
    const resp = allRequests.rows.map((row) => {
      return {
        id: row.id,
        endpoint: row.endpoint,
        method: row.method,
        body: row.body,
        userAgent: row.user_agent,
        isFrontend: row.is_frontend,
        timestamp: new Date(row.timestamp),
      };
    });

    res.status(200).json(resp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
  logRequest(req, res, start);
}

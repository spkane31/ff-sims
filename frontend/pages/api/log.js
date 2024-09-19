// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { logRequest, pool } from "../../db/db";

export default async function log(req, res) {
  // if the environment variable NODE_ENV is not prod, don't log the request
  if (process.env.NODE_ENV !== "production") {
    console.log(
      `[INFO] NODE_ENV is not production (${process.env.NODE_ENV}), skipping page logging`
    );
    return;
  }
  try {
    const client = await pool.connect();
    // insert endpoint, method, and timestamp into log table, parsing the first two from the body of the req
    const query = `
      INSERT INTO requests (endpoint, method, timestamp, is_frontend)
      VALUES ($1, $2, $3, true)
      RETURNING id;
    `;
    const resp = await client.query(query, [
      req.body.endpoint,
      req.body.method,
      new Date(),
    ]);
    client.release();
    res.status(200).json(resp);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

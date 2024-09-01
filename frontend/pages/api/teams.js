// Next.js API route support: https://nextjs.org/docs/api-routes/introduction
import { Pool } from "pg/lib";
import { config } from "../../config";
const pool = new Pool(config);

export default async function teams(req, res) {
  const query = `SELECT espn_id AS id, owner FROM teams;`;
  try {
    const client = await pool.connect();
    const resp = await client.query(query);
    const respIDAsInt = resp.rows.map((row) => {
      return {
        id: parseInt(row.id),
        owner: row.owner,
      };
    });
    res.status(200).json(respIDAsInt);
  } catch (err) {
    res.status(500).json({
      message: err.message,
    });
  }
}

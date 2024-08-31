// Next.js API route support: https://nextjs.org/docs/api-routes/introduction

import schedule from "../../data/schedule.json";

export default function hello(req, res) {
  res.status(200).json(schedule);
}

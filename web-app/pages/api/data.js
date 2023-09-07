// not the greatest (or even good) API design but hey it works

import Cors from "cors";

// Initializing the cors middleware
// You can read more about the available options here: https://github.com/expressjs/cors#configuration-options
const cors = Cors({
  methods: ["POST", "GET", "HEAD"],
});

// Helper method to wait for a middleware to execute before continuing
// And to throw an error when an error happens in a middleware
function runMiddleware(req, res, fn) {
  return new Promise((resolve, reject) => {
    fn(req, res, (result) => {
      if (result instanceof Error) {
        return reject(result);
      }

      return resolve(result);
    });
  });
}

export default async function handler(req, res) {
  // Run the middleware
  await runMiddleware(req, res, cors);

  // Rest of the API logic
  res.json({
    final_standings: [
      [
        "Team 1",
        ".12",
        ".02",
        ".03",
        ".04",
        ".05",
        ".06",
        ".07",
        ".08",
        ".09",
        ".10",
      ],
      [
        "Team 2",
        ".13",
        ".02",
        ".03",
        ".04",
        ".05",
        ".06",
        ".07",
        ".08",
        ".09",
        ".10",
      ],
    ],
  });
}

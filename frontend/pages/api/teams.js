// Next.js API route support: https://nextjs.org/docs/api-routes/introduction

export default function teams(req, res) {
  res.status(200).json([
    {
      id: 12,
      owner: "Ethan Moran",
    },
    {
      id: 10,
      owner: "Jack Aldridge",
    },
    {
      id: 3,
      owner: "Nick Toth",
    },
    {
      id: 7,
      owner: "Josh Doepker",
    },
    {
      id: 1,
      owner: "Kyle Burns",
    },
    {
      id: 6,
      owner: "Sean  Kane",
    },
    {
      id: 9,
      owner: "Mitch Lichtinger",
    },
    {
      id: 11,
      owner: "Nick Dehaven",
    },
    {
      id: 5,
      owner: "Kevin Dailey",
    },
    {
      id: 4,
      owner: "Connor Brand",
    },
  ]);
}

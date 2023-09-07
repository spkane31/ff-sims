// not the greatest (or even good) API design but hey it works

export default function data(req, res) {
  res.status(200).json({
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

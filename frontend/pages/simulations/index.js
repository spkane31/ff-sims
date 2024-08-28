import * as React from "react";

import {
  Table,
  TableHead,
  TableCell,
  TableRow,
  TableBody,
} from "@mui/material";

import schedule from "../../data/schedule.json";
import team_avgs from "../../data/team_avgs.json";

export default function Home() {
  return (
    <div>
      <h1>Simulations</h1>
      <TeamData />
      <Schedule />
    </div>
  );
}

const Schedule = () => {
  return (
    <ul>
      {schedule.map((week, weekIdx) => (
        <li key={weekIdx}>
          <h2>Week {weekIdx + 1}</h2>
          <ul>
            {week.map((game, gameIdx) => (
              <li key={gameIdx}>
                {game.away_team_owner} @ {game.home_team_owner}
              </li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  );
};

const TeamData = () => {
  return (
    <>
      <h1>Team Data</h1>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>Team Name</TableCell>
            <TableCell>Average</TableCell>
            <TableCell>Standard Deviation</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {Object.entries(team_avgs).map(([teamName, { average, std_dev }]) => (
            <TableRow key={teamName}>
              <TableCell>{teamName}</TableCell>
              <TableCell>{average}</TableCell>
              <TableCell>{std_dev}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </>
  );
};

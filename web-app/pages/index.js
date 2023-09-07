import Head from "next/head";
import BasicTable from "../components/table";
import styles from "../styles/Home.module.css";
import * as React from "react";

export default function Home() {
  const [data, setData] = React.useState([]);

  React.useEffect(() => {
    fetch("http://localhost:3000/api/data", { method: "GET" })
      .then((res) => res.json())
      .then((data) => setData(data));
  }, []);

  return (
    <div className={styles.container}>
      <Head>
        <title>The League Fantasy Football</title>
      </Head>

      <main className={styles.main}>
        <h1>The League Fantasy Football Analytics</h1>
        {data.length === 0 ? (
          <></>
        ) : (
          <BasicTable
            columns={[
              "Team",
              "1st",
              "2nd",
              "3rd",
              "4th",
              "5th",
              "6th",
              "7th",
              "8th",
              "9th",
              "Hot Dog",
            ]}
            data={data.final_standings}
          />
        )}
        <h2>Playoff Seed Probabilities</h2>

        <BasicTable
          columns={["Team", "1st", "2nd", "3rd", "4th", "5th", "6th"]}
          data={data.playoff_seeds}
        />

        <h2>This Weeks Games</h2>

        <BasicTable
          columns={[
            "Home Team",
            "Home Projected",
            "Away Projected",
            "Away Team",
          ]}
        />

        <h2>Points Left on the Bench</h2>
        <BasicTable
          columns={["Team Name", "Points"]}
          data={data.points_left_on_bench}
        />

        <h3>Perfect Rosters</h3>
        <p>Perfect roster by Kyle Burns in week 6</p>
        <p>Perfect roster by Kevin Dailey in week 7</p>
        <p>Perfect roster by Josh Doepker in week 8</p>
        <p>Perfect roster by Kevin Dailey in week 8</p>
        <p>Perfect roster by jack aldridge in week 9</p>
        <p>Perfect roster by Sean Kane in week 10</p>
        <p>Perfect roster by Ethan Moran in week 10</p>
        <p>Perfect roster by Kevin Dailey in week 12</p>
        <p>Perfect roster by Nick DeHaven in week 12</p>
        <p>Perfect roster by nick toth in week 13</p>
        <p>Perfect roster by Kevin Dailey in week 14</p>

        <h2>ESPN Accuracy</h2>

        <h3>By Team</h3>

        <BasicTable
          columns={["Team", "Total Difference", "Per Game Difference"]}
          data={
            data.espn_accuracy !== undefined &&
            data.espn_accuracy.by_team !== undefined
              ? data.espn_accuracy.by_team
              : undefined
          }
        />

        <h3>By Position</h3>

        <BasicTable
          columns={["Position", "Average % Difference", "Std. Dev"]}
          data={
            data.espn_accuracy !== undefined &&
            data.espn_accuracy.by_position !== undefined
              ? data.espn_accuracy.by_position
              : undefined
          }
        />

        <h2>Waiver Wires</h2>

        <BasicTable
          columns={[
            "Players Added",
            "Players Dropped",
            "Players Added Points",
            "Players Dropped Points",
          ]}
        />

        <h2>Trades</h2>

        <BasicTable
          columns={[
            "Team 1 Gets",
            "Team 2 Gets",
            "Team 1 Points Scored",
            "Team 2 Points Scored",
          ]}
        />

        <h2>Draft</h2>

        <BasicTable columns={["Pick #", "Player", "Position", "Points"]} />
      </main>
    </div>
  );
}

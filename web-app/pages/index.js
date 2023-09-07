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

        <h2>ESPN Accuracy</h2>

        <h3>By Team</h3>

        <BasicTable columns={["Team", "Point Differential"]} />

        <h3>By Position</h3>

        <BasicTable columns={["Position", "Point Differential"]} />

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

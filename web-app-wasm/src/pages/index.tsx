import type { NextPage } from "next";
import Head from "next/head";
import styles from "../styles/Home.module.css";
import { WASMExample } from "../components/WASMExample";

const Home: NextPage = () => {
  return (
    <div className={styles.container}>
      <Head>
        <title>The League Fantasy Football History</title>
        <meta
          name="description"
          content="The League Fantasy Football History"
        />
        <link rel="icon" href="/favicon.ico" />
      </Head>

      <main className={styles.main}>
        <div className={styles.wasm}>
          <WASMExample />
        </div>
      </main>
    </div>
  );
};

export default Home;

import { useContext } from "react";
import { WASMContext } from "../context/WASM";
import styles from "../styles/Home.module.css";

export const WASMExample = () => {
  const ctx = useContext(WASMContext);

  if (!ctx.wasm) {
    return <>...WASM not available...</>;
  }

  return (
    <div>
      Time
      <input />
      <br />
      Distance
      <input />
      <br />
      Pace
      <input />
      <br />
      <div className={styles.displaylinebreak}>{ctx.wasm.simulate()}</div>
    </div>
  );
};

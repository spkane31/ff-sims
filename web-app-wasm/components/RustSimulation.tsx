import type { SimulationModuleExports } from "../wasm";
import dynamic from "next/dynamic";

// Properties for a separate time
interface RustComponentProps {
  number: Number;
}

const RustSimulation = dynamic({
  loader: async () => {
    // Import the wasm module
    // @ts-ignore
    const exports = (await import("../add.wasm")) as SimulationModuleExports;
    const { simulation: simulation } = exports;

    // Return a React component that calls the add_one method on the wasm module
    return () => (
      <div>
        <>Simulation</>
        <>{simulation()}</>
        <br />
        <>End</>
      </div>
    );
  },
});

export default RustSimulation;

import type { MinusModuleExports } from "../wasm";
import dynamic from "next/dynamic";

interface RustComponentProps {
  number: Number;
}

const RustComponent = dynamic({
  loader: async () => {
    // Import the wasm module
    // @ts-ignore
    const exports = (await import("../add.wasm")) as MinusModuleExports;
    const { minus_one: minusOne } = exports;

    // Return a React component that calls the add_one method on the wasm module
    return ({ number }: RustComponentProps) => (
      <div>
        <>{minusOne(number)}</>
      </div>
    );
  },
});

export default RustComponent;

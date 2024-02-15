import { useRouter } from "next/router";
import Link from "next/link";
import RustComponent from "../components/RustComponent";
import RustSimulation from "../components/RustSimulation";
import RustComponentMinus from "../components/RustComponentMinus";

export default function Page() {
  const { query } = useRouter();
  const number = parseInt(query.number as string) || 30;

  return (
    <div>
      {/* <Link href={`/?number=${number - 1}`}>-</Link> */}
      {/* <RustComponent number={number} /> */}
      {/* <RustComponentMinus number={number} /> */}

      <RustSimulation />
      {/* <Link href={`/?number=${number + 1}`}>+</Link> */}
    </div>
  );
}

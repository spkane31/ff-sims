import Link from "next/link";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { MoreIcon } from "./icons";
import { FOCUS_RING } from "@/components/design-system/focus-ring";

interface MoreMenuItemResolved {
  label: string;
  href: string;
}

interface MoreMenuProps {
  items: MoreMenuItemResolved[];
}

export default function MoreMenu({ items }: MoreMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="More"
          className={`flex min-h-11 min-w-11 items-center justify-center rounded-md ${FOCUS_RING}`}
        >
          <MoreIcon className="h-5 w-5" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {items.map((item) => (
          <DropdownMenuItem key={item.href} asChild>
            <Link href={item.href}>{item.label}</Link>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

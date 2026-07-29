import { useRouter } from "next/router";
import { useLeagues } from "@/hooks/useLeagues";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

interface LeagueSwitcherProps {
  leagueId: string;
}

/**
 * Real league context/switcher for the top bar. Only rendered when a
 * `leagueId` is present in the route (matches the previous `Header.tsx`'s
 * behavior of only showing league context when one exists).
 */
export default function LeagueSwitcher({ leagueId }: LeagueSwitcherProps) {
  const router = useRouter();
  const { leagues, isLoading } = useLeagues();

  if (isLoading) {
    return <Skeleton className="h-11 w-40 rounded-md" />;
  }

  return (
    <Select
      value={leagueId}
      onValueChange={(newId) => router.push(`/league/${newId}`)}
    >
      <SelectTrigger className="min-h-11 max-w-[10rem] sm:max-w-xs">
        <SelectValue placeholder="Select a league" />
      </SelectTrigger>
      <SelectContent>
        {leagues.map((league) => (
          <SelectItem key={league.id} value={String(league.id)}>
            {league.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

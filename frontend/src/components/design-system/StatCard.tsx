import { Card, CardContent } from "@/components/ui/card";

interface StatCardProps {
  label: string;
  value: string;
  detail?: string;
}

export default function StatCard({ label, value, detail }: StatCardProps) {
  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>
          {label}
        </p>
        <p className="mt-1 text-2xl font-bold" style={{ color: "var(--text-primary)" }}>
          {value}
        </p>
        {detail && (
          <p className="mt-1 text-sm" style={{ color: "var(--text-secondary)" }}>
            {detail}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

import { Button } from "@/components/ui/button";

interface ErrorStateProps {
  message: string;
  onRetry?: () => void;
}

export default function ErrorState({ message, onRetry }: ErrorStateProps) {
  return (
    <div
      role="alert"
      className="flex flex-col items-start gap-3 rounded-md border px-4 py-4 sm:flex-row sm:items-center sm:justify-between"
      style={{
        borderColor: "var(--status-danger-fg)",
        backgroundColor: "var(--status-danger-bg)",
        color: "var(--status-danger-fg)",
      }}
    >
      <p className="text-sm font-medium">{message}</p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Retry
        </Button>
      )}
    </div>
  );
}

import { cn } from "@/lib/utils";

type StatusDotProps = {
  color: string;
  pulse?: boolean;
};

export function StatusDot({ color, pulse }: StatusDotProps) {
  return (
    <span className="relative flex h-2 w-2">
      {pulse && (
        <span
          className={cn(
            "absolute inline-flex h-full w-full animate-ping rounded-full opacity-60",
            color,
          )}
        />
      )}
      <span className={cn("relative inline-flex h-2 w-2 rounded-full", color)} />
    </span>
  );
}

export const STATE_DOT_COLORS: Record<string, string> = {
  INITED: "bg-muted-foreground/50",
  RUNNING: "bg-warning",
  DONE: "bg-success",
  FAILED: "bg-destructive",
  CANCELLED: "bg-muted-foreground/30",
};

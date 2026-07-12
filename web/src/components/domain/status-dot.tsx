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
            "absolute inline-flex h-full w-full animate-ping rounded-full opacity-75",
            color,
          )}
        />
      )}
      <span className={cn("relative inline-flex h-2 w-2 rounded-full", color)} />
    </span>
  );
}

export const STATE_DOT_COLORS: Record<string, string> = {
  INITED: "bg-zinc-500",
  RUNNING: "bg-amber-500",
  DONE: "bg-green-500",
  FAILED: "bg-red-500",
  CANCELLED: "bg-zinc-400",
};

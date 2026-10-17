import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  // whitespace-nowrap：徽章在窄列里会被 flex 挤到逐字换行（「启用中」竖排），任何状态标签都不该折行
  "inline-flex items-center whitespace-nowrap rounded-md border px-2 py-0.5 text-xs font-medium transition-colors duration-150",
  {
    variants: {
      variant: {
        default:
          "border-transparent bg-primary text-primary-foreground",
        secondary:
          "border-border/60 bg-muted text-muted-foreground",
        destructive:
          "border-transparent bg-destructive/12 text-destructive dark:bg-destructive/20",
        outline: "border-border text-foreground",
        success:
          "border-transparent bg-success/12 text-success dark:bg-success/20",
        warning:
          "border-transparent bg-warning/12 text-warning dark:bg-warning/20",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge, badgeVariants };
